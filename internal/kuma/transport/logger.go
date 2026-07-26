package transport

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Logger adapts the Socket.IO library's logger onto tflog.
//
// This is not cosmetic: the library's default logger writes to stdout, and a
// Terraform provider speaks the plugin protocol over stdout. Anything printed
// there corrupts the handshake, so every log line has to be redirected.
type Logger struct {
	ctx context.Context
}

func (l *Logger) Debugf(format string, v ...any) {
	tflog.Debug(l.ctx, fmt.Sprintf(format, v...))
}

func (l *Logger) Infof(format string, v ...any) {
	tflog.Debug(l.ctx, fmt.Sprintf(format, v...))
}

func (l *Logger) Warnf(format string, v ...any) {
	tflog.Warn(l.ctx, fmt.Sprintf(format, v...))
}

func (l *Logger) Errorf(format string, v ...any) {
	// Deliberately logged at warn level: the library reports recoverable
	// conditions (an ack with no registered callback, an event with no
	// handler) through Errorf, and those are normal for this client since it
	// only subscribes to the events it needs.
	tflog.Warn(l.ctx, fmt.Sprintf(format, v...))
}

// NewLogger returns a logger that writes through tflog with the given context.
func NewLogger(ctx context.Context) *Logger {
	return &Logger{ctx: ctx}
}
