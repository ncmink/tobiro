package main

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v3/pkg/application"

	"tobiro/internal/domain"
	"tobiro/internal/logging"
	"tobiro/internal/session"
)

type uiBridge struct {
	pendingMu sync.Mutex
	pending   map[string]chan promptAnswer
}

type promptAnswer struct {
	code      string
	cancelled bool
}

func newUIBridge() *uiBridge {
	return &uiBridge{pending: make(map[string]chan promptAnswer)}
}

func (b *uiBridge) Notify(event string, payload any) {
	app := application.Get()
	if app == nil {
		return
	}
	app.Event.Emit(event, payload)
}

func (b *uiBridge) present() {
	presentMainWindow()
}

func (b *uiBridge) requestConnect(profileID string) {
	b.present()
	b.Notify(domain.EventConnectRequested, ConnectRequestView{ProfileID: profileID})
}

const nativeFillCountdownSeconds = 5

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

func (b *uiBridge) AwaitNativeFill(ctx context.Context, req domain.NativeFillRequest) error {
	requestID, answers, release := b.await()
	defer release()

	b.present()
	b.Notify(domain.EventNativeFillPending, NativeFillPendingView{
		RequestID:        requestID,
		ProfileID:        req.ProfileID,
		ConnectionName:   req.ProfileName,
		HasPassword:      req.HasPassword,
		CountdownSeconds: nativeFillCountdownSeconds,
	})

	_, err := wait(ctx, answers)
	return err
}

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

func (b *uiBridge) onSessionChanged(s domain.VPNSession) {
	b.Notify(domain.EventSessionChanged, s.View())
}

func (b *uiBridge) onLogAppended(entry logging.Entry) {
	b.Notify(domain.EventLogAppended, entry)
}

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
