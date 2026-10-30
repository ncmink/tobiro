package domain

// NativeFillRequest pauses before typing into a vendor login window. The UI
// shows a countdown; the backend blocks until the user proceeds, cancels, or
// the UI completes the wait.
type NativeFillRequest struct {
	ProfileID   string
	ProfileName string
	HasPassword bool
}
