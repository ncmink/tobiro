package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	bctx "github.com/ncmink/biebie-protocol/context"
	"github.com/ncmink/biebie-protocol/deeplink"
	"github.com/google/uuid"
	"github.com/wailsapp/wails/v2/pkg/options"

	"tobiro/internal/peer"
)

// This file holds everything to do with the other biebie.net applications:
// the local endpoint they call, and the deep links they open.
//
// Both arrive at the same place. Whether Biebie Kube found this application
// already running and spoke to it over the socket, or launched it with a URL,
// the outcome is identical: the window comes forward at that connection and the
// user chooses an account. Neither route can make this application sign in by
// itself.

// pendingConnect holds an incoming request until the frontend collects it.
//
// It exists because a deep link can launch this application, arriving before
// there is a window to raise or a frontend to notify. Holding the request and
// letting the UI ask for it on mount covers that without the request being lost.
//
// It is one slot, not a queue. Every request says the same thing — show me this
// connection — and only the most recent one is what the user is waiting for.
// Replaying earlier ones would move the window somewhere they stopped looking.
type pendingConnect struct {
	mu        sync.Mutex
	profileID string
}

func (p *pendingConnect) set(profileID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.profileID = profileID
}

func (p *pendingConnect) take() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	profileID := p.profileID
	p.profileID = ""
	return profileID
}

// startPeerEndpoint opens the endpoint other biebie.net applications call.
//
// Failing to bind is logged and nothing more. It normally means a second Biebie
// Access is already running and already answering, and this window is perfectly
// usable without an endpoint of its own: talking to Biebie Kube is something
// this application offers, not something it depends on.
func (a *App) startPeerEndpoint() {
	endpoint, err := peer.New(peer.Options{
		Profiles:           a.profiles,
		Sessions:           a.sessions,
		Access:             a.access,
		Log:                a.log,
		OnConnectRequested: a.requestConnect,
		OnError: func(err error) {
			a.log.Warn("Local endpoint: %v", err)
		},
	})
	if err != nil {
		a.log.Warn("Cannot address a local endpoint: %v", err)
		return
	}
	if err := endpoint.Start(); err != nil {
		a.log.Warn("Not answering other Biebie applications: %v", err)
		return
	}

	a.peer = endpoint
	a.log.Info("Answering other Biebie applications on %s", endpoint.Address())
}

// startPeerNotifier announces connection state changes to Biebie Kube.
//
// It closes the loop in the other direction. A cluster that refused to connect
// because the customer network was down would otherwise wait for someone to
// click again; told that the tunnel is up, it retries on its own.
//
// The notifier is registered as a second observer rather than replacing the one
// the window uses. Both audiences see every change.
func (a *App) startPeerNotifier() {
	notifier, err := peer.NewNotifier()
	if err != nil {
		a.log.Warn("Not announcing connection changes to Biebie Kube: %v", err)
		return
	}
	a.notifier = notifier
	a.sessions.AddObserver(notifier.Observe)
}

// shutdown releases the endpoint.
//
// The socket must not outlive the process: a caller that connected to a stale
// one would sit waiting on a listener nobody is reading, instead of falling
// back to the deep link that would have launched this application.
func (a *App) shutdown(context.Context) {
	if a.peer != nil {
		_ = a.peer.Close()
	}
	if a.notifier != nil {
		a.notifier.Close()
	}
	// SSH tunnels live in this process, so closing the window has to close
	// them. A listener left bound after the application exits would be a local
	// port that answers and then goes nowhere.
	a.tunnels.Shutdown()
}

// requestConnect records a request from another application and nudges the UI.
//
// The value is stored rather than passed in the event, so there is one way to
// collect it whether the request arrived at a running window or launched the
// application. The event only says "there is something to collect".
func (a *App) requestConnect(profileID string) {
	a.pendingConnect.set(profileID)
	a.bridge.requestConnect(profileID)
}

// OpenInKube issues a handoff ticket and opens Biebie Kube at the cluster the
// engineer named. Tobiro never guesses a cluster mapping: every
// identifier in the context was chosen on this screen.
func (a *App) OpenInKube(input OpenKubeInput) (OpenKubeResultView, error) {
	if a.peer == nil {
		return OpenKubeResultView{}, fmt.Errorf("Tobiro is not answering other applications on this machine")
	}
	profile, err := a.profiles.Get(input.ProfileID)
	if err != nil {
		return OpenKubeResultView{}, a.describe(err)
	}

	clusterID := strings.TrimSpace(input.ClusterID)
	clusterName := strings.TrimSpace(input.ClusterName)
	customerID := strings.TrimSpace(input.CustomerID)
	customerName := strings.TrimSpace(input.CustomerName)
	if clusterID == "" || clusterName == "" {
		return OpenKubeResultView{}, fmt.Errorf("a cluster id and name are required")
	}
	if customerID == "" || customerName == "" {
		return OpenKubeResultView{}, fmt.Errorf("a customer id and name are required")
	}

	now := time.Now().UTC()
	received := bctx.BiebieContext{
		ContextID:       "ctx_" + uuid.NewString(),
		CustomerID:      customerID,
		CustomerName:    customerName,
		EnvironmentID:   strings.TrimSpace(input.EnvironmentID),
		EnvironmentName: strings.TrimSpace(input.EnvironmentName),
		EnvironmentKind: bctx.Environment(strings.TrimSpace(input.EnvironmentKind)),
		AccessProfileID: profile.ID,
		ClusterID:       clusterID,
		ClusterName:     clusterName,
		Server:          strings.TrimSpace(input.Server),
		CreatedAt:       now,
		ExpiresAt:       now.Add(60 * time.Second),
	}

	handoffID, err := a.peer.CreateHandoff(a.ctx, bctx.AppKube, received)
	if err != nil {
		return OpenKubeResultView{}, a.describe(err)
	}
	link, err := deeplink.OpenKube(handoffID)
	if err != nil {
		return OpenKubeResultView{}, a.describe(err)
	}

	result := OpenKubeResultView{HandoffID: handoffID, Link: link}
	if peer.DeliverKubeLink(link) {
		result.Delivered = true
		a.log.Info("Opened cluster %q in Biebie Kube over IPC", clusterName)
		return result, nil
	}
	if err := peer.LaunchKubeLink(link); err != nil {
		return result, fmt.Errorf("could not open Biebie Kube: %w", err)
	}
	result.Launched = true
	a.log.Info("Launched Biebie Kube for cluster %q", clusterName)
	return result, nil
}

// TakeConnectRequest returns a request from another biebie.net application, and
// forgets it.
//
// The frontend calls this when it mounts and whenever it is told a request has
// arrived. An empty profile id means there is nothing waiting.
func (a *App) TakeConnectRequest() ConnectRequestView {
	return ConnectRequestView{ProfileID: a.pendingConnect.take()}
}

// handleDeepLink honours a tobiro:// URL.
//
// This is macOS's route: the running application is handed the URL directly, so
// clicking "Connect with Tobiro" in Biebie Kube reaches here even when
// the socket was never bound.
func (a *App) handleDeepLink(raw string) {
	profileID, err := connectTarget(raw)
	if err != nil {
		// The URL is logged, never the reason it was refused verbatim from a
		// query string — the protocol's parser rejects links carrying credential
		// material, and echoing one back would defeat that.
		a.log.Warn("Ignoring deep link %s: %v", deeplink.Scheme(raw), err)
		return
	}

	resolved, err := a.profiles.Resolve(profileID)
	if err != nil {
		// The window still comes forward. This application was very likely just
		// launched by someone clicking a button in Biebie Kube, and a launch
		// that shows nothing looks like the click did nothing at all. The
		// connection list is where they can see for themselves that what their
		// cluster refers to is not one of these.
		a.log.Warn("A deep link asked for connection %q, which does not exist here: %v", profileID, err)
		a.bridge.present()
		return
	}
	// The resolved identifier is what the UI is given, never the reference as
	// written: the window has to open a connection, and a name is not something
	// it can look one up by.
	a.requestConnect(resolved.ID)
}

// handleSecondInstance honours a deep link on Windows and Linux, where the URL
// is delivered by launching the application again rather than to the process
// that already holds the window.
func (a *App) handleSecondInstance(data options.SecondInstanceData) {
	for _, arg := range data.Args {
		if deeplink.Scheme(arg) == deeplink.SchemeAccess {
			a.handleDeepLink(arg)
			return
		}
	}
	// Launched again with nothing to act on, which is a user double-clicking the
	// application. Showing the window they were reaching for is the whole job.
	a.bridge.present()
}

// connectTarget reads the profile a connect link names.
func connectTarget(raw string) (string, error) {
	request, err := deeplink.ParseConnect(raw)
	if err != nil {
		return "", err
	}
	if profileID := strings.TrimSpace(request.ProfileID); profileID != "" {
		return profileID, nil
	}
	return "", errors.New("no profile named")
}
