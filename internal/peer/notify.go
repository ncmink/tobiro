package peer

import (
	"context"
	"sync"
	"time"

	bctx "github.com/ncmink/biebie-protocol/context"
	"github.com/ncmink/biebie-protocol/ipc"

	"tobiro/internal/domain"
)

// notifyTimeout bounds a notification.
//
// Announcing a state change must never slow down the connection that caused it,
// and the peer is on the same machine — if it has not answered in this long, it
// is not going to.
const notifyTimeout = 2 * time.Second

// Notifier tells Biebie Kube when a connection's state changes.
//
// Without it, Biebie Kube learns about a tunnel coming up only by asking again.
// A cluster that refused to connect because the customer network was down would
// sit there until someone clicked, having no way to know the wait was over. This
// closes that gap in the direction the information travels.
type Notifier struct {
	rpc *ipc.Client

	// The lock covers both fields below and the send. Observe is called from
	// whichever goroutine moved the session — a connect sequence, or the status
	// poller — so none of this is single-threaded.
	mu sync.Mutex

	// last remembers what was sent for each profile, so a state that has not
	// changed is not announced again. Vendor clients are polled, which means the
	// same state arrives over and over.
	last   map[string]bctx.AccessState
	closed bool

	send chan bctx.AccessSessionChanged
}

// NewNotifier prepares a notifier addressed at Biebie Kube.
func NewNotifier() (*Notifier, error) {
	endpoint, err := ipc.KubeEndpoint()
	if err != nil {
		return nil, err
	}
	return NewNotifierFor(endpoint), nil
}

// NewNotifierFor addresses a specific endpoint, for tests.
func NewNotifierFor(endpoint ipc.Endpoint) *Notifier {
	n := &Notifier{
		rpc:  ipc.NewClient(endpoint).WithTimeout(notifyTimeout),
		last: make(map[string]bctx.AccessState),

		// Buffered and served by one goroutine, because these arrive from the
		// session manager's own notification path. Dialling a socket there would
		// hold up the connection sequence that produced the change.
		send: make(chan bctx.AccessSessionChanged, 32),
	}
	go n.loop()
	return n
}

// Observe is a session observer. It is registered alongside the one that updates
// this application's window, and reports the same changes outward.
//
// A profile identifier and a coarse state; nothing else crosses.
func (n *Notifier) Observe(s domain.VPNSession) {
	state := accessState(s.State)

	// Held across the send, which is safe because the send cannot block: the
	// default case below takes over the moment the queue is full.
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.closed || n.last[s.ProfileID] == state {
		return
	}
	n.last[s.ProfileID] = state

	select {
	case n.send <- bctx.AccessSessionChanged{ProfileID: s.ProfileID, State: state}:
	default:
		// The queue is full, so Biebie Kube is not keeping up. Dropping is
		// right: it asks for the current state when it next needs it, and
		// blocking here would stall the tunnel this change describes.
	}
}

func (n *Notifier) loop() {
	for event := range n.send {
		ctx, cancel := context.WithTimeout(context.Background(), notifyTimeout)
		// The error is deliberately unexamined. Biebie Kube not running is the
		// ordinary case, and there is no version of this that the user should
		// see: nothing is broken, and nothing was lost.
		_ = n.rpc.Call(ctx, ipc.MethodAccessSessionChanged, event, nil)
		cancel()
	}
}

// Close stops the notifier. It is safe to call more than once, and safe to call
// while a status poll is still reporting.
func (n *Notifier) Close() {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return
	}
	n.closed = true
	close(n.send)
}
