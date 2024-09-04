package cmd
type MessageCallback interface {
	OnMessageReceived(message string)
}