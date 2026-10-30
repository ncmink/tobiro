package main

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"tobiro/internal/domain"
	"tobiro/internal/logging"
	"tobiro/internal/session"
)

// uiBridge connects the Go core to the Vue UI over Wails runtime events.
//
// It implements vpn.Prompter and sshtunnel.Prompter, which is how a connection
// attempt pauses to ask the user for a one-time code or to vouch for an unknown
// SSH server. The answer lives in a channel for the duration of the connect
// call and is never stored or logged.
type uiBridge struct {
	mu  sync.RWMutex
	ctx context.Context

	pendingMu sync.Mutex
	pending   map[string]chan promptAnswer
}

// promptAnswer carries whatever the dialog collected. A one-time code needs the
// text; a host key needs only whether the user said yes, which is the absence
// of a cancellation.
type promptAnswer struct {
	code      string
	cancelled bool
}

func newUIBridge() *uiBridge {
	return &uiBridge{pending: make(map[string]chan promptAnswer)}
}

// attach stores the Wails context once the app has started.
func (b *uiBridge) attach(ctx context.Context) {
	b.mu.Lock()
	b.ctx = ctx
	b.mu.Unlock()
}

func (b *uiBridge) context() (context.Context, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.ctx, b.ctx != nil
}

// Notify emits an event to the frontend.
func (b *uiBridge) Notify(event string, payload any) {
	if ctx, ok := b.context(); ok {
		wailsruntime.EventsEmit(ctx, event, payload)
	}
}

// present brings the window forward.
//
// It has to be explicit: the user clicked a button in another application and
// expects this one to appear, which will not happen on its own when it is
// already running behind other windows, or minimised — the case this exists for.
func (b *uiBridge) present() {
	ctx, ok := b.context()
	if !ok {
		// Startup has not finished, so there is no window yet. The window Wails
		// is about to create is the one the user asked for.
		return
	}
	wailsruntime.WindowUnminimise(ctx)
	wailsruntime.WindowShow(ctx)
}

// requestConnect surfaces this application and tells the UI a request is
// waiting to be collected.
func (b *uiBridge) requestConnect(profileID string) {
	b.present()
	b.Notify(domain.EventConnectRequested, ConnectRequestView{ProfileID: profileID})
}

const nativeFillCountdownSeconds = 5

// RequestOTP asks the UI for a one-time code and blocks until the user answers,
// cancels, or the connect context is cancelled.
func (b *uiBridge) RequestOTP(ctx context.Context, req domain.InteractionRequest) (string, error) {
	requestID, answers, release := b.await()
	defer release()

	b.Notify(domain.EventOTPRequired, OTPRequestView{
		RequestID: requestID,
		ProfileID: req.ProfileID,
		Prompt:    defaultText(req.Prompt, "One-time code"),
		Message:   req.Message,
	})

	answer, err := wait(ctx, answers)
	if err != nil {
		return "", err
	}
	return answer.code, nil
}

// AwaitNativeFill blocks until the user proceeds with login fill, cancels, or
// the connect context ends.
func (b *uiBridge) AwaitNativeFill(ctx context.Context, req domain.NativeFillRequest) error {
	requestID, answers, release := b.await()
	defer release()

	b.present()
	b.Notify(domain.EventNativeFillPending, NativeFillPendingView{
		RequestID:          requestID,
		ProfileID:          req.ProfileID,
		ConnectionName:     req.ProfileName,
		HasPassword:        req.HasPassword,
		CountdownSeconds:   nativeFillCountdownSeconds,
	})

	_, err := wait(ctx, answers)
	if err != nil {
		return err
	}
	return nil
}

// ConfirmHostKey asks the user to vouch for an SSH server this machine has not
// met before, and blocks until they answer.
//
// It is a separate prompt from the OTP one because the question is different in
// kind: an OTP dialog asks for something the user has, and this one asks them
// to decide something. The fingerprint has to be shown in full and compared
// against what the server's owner published, which no default can do for them.
func (b *uiBridge) ConfirmHostKey(ctx context.Context, req domain.HostKeyRequest) (bool, error) {
	requestID, answers, release := b.await()
	defer release()

	b.present()
	b.Notify(domain.EventHostKeyReview, HostKeyRequestView{
		RequestID:   requestID,
		ProfileID:   req.ProfileID,
		Host:        req.Host,
		Algorithm:   req.Algorithm,
		Fingerprint: req.Fingerprint,
		KnownHosts:  req.KnownHosts,
	})

	if _, err := wait(ctx, answers); err != nil {
		if errors.Is(err, domain.ErrInteractionCancelled) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// await registers a pending prompt and returns the channel its answer arrives
// on, plus the cleanup that stops the UI answering a prompt nobody waits for.
func (b *uiBridge) await() (string, chan promptAnswer, func()) {
	requestID := uuid.NewString()
	answers := make(chan promptAnswer, 1)

	b.pendingMu.Lock()
	b.pending[requestID] = answers
	b.pendingMu.Unlock()

	return requestID, answers, func() {
		b.pendingMu.Lock()
		delete(b.pending, requestID)
		b.pendingMu.Unlock()
	}
}

func wait(ctx context.Context, answers chan promptAnswer) (promptAnswer, error) {
	select {
	case <-ctx.Done():
		return promptAnswer{}, fmt.Errorf("the connection attempt was cancelled")
	case answer := <-answers:
		if answer.cancelled {
			return promptAnswer{}, domain.ErrInteractionCancelled
		}
		return answer, nil
	}
}

// proceedInteraction delivers a proceed signal from the UI to a waiting prompt.
func (b *uiBridge) proceedInteraction(requestID string) error {
	b.pendingMu.Lock()
	ch, ok := b.pending[requestID]
	b.pendingMu.Unlock()
	if !ok {
		return fmt.Errorf("this prompt is no longer waiting for an answer")
	}
	select {
	case ch <- promptAnswer{}:
		return nil
	default:
		return fmt.Errorf("this prompt already received an answer")
	}
}

// resolveOTP delivers a code from the UI to the waiting adapter.
func (b *uiBridge) resolveOTP(requestID, code string) error {
	b.pendingMu.Lock()
	ch, ok := b.pending[requestID]
	b.pendingMu.Unlock()
	if !ok {
		return fmt.Errorf("this prompt is no longer waiting for an answer")
	}
	select {
	case ch <- promptAnswer{code: code}:
		return nil
	default:
		return fmt.Errorf("this prompt already received an answer")
	}
}

// cancelInteraction abandons a pending prompt.
func (b *uiBridge) cancelInteraction(requestID string) error {
	b.pendingMu.Lock()
	ch, ok := b.pending[requestID]
	b.pendingMu.Unlock()
	if !ok {
		return nil
	}
	select {
	case ch <- promptAnswer{cancelled: true}:
	default:
	}
	return nil
}

// onSessionChanged streams session updates to the UI.
func (b *uiBridge) onSessionChanged(s domain.VPNSession) {
	b.Notify(domain.EventSessionChanged, s.View())
}

// onLogAppended streams redacted log entries to the UI.
func (b *uiBridge) onLogAppended(entry logging.Entry) {
	b.Notify(domain.EventLogAppended, entry)
}

// wire installs the bridge's callbacks on the logger and session manager.
func (b *uiBridge) wire(log *logging.Logger, sessions *session.Manager) {
	log.SetSink(b.onLogAppended)
	sessions.AddObserver(b.onSessionChanged)
}

func defaultText(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
