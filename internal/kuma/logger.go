package kuma

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// logger adapts the Socket.IO library's logger onto tflog.
//
// This is not cosmetic: the library's default logger writes to stdout, and a
// Terraform provider speaks the plugin protocol over stdout. Anything printed
// there corrupts the handshake, so every log line has to be redirected.
type logger struct {
	ctx context.Context
}

func (l *logger) Debugf(format string, v ...any) {
	tflog.Debug(l.ctx, fmt.Sprintf(format, v...))
}

func (l *logger) Infof(format string, v ...any) {
	tflog.Debug(l.ctx, fmt.Sprintf(format, v...))
}

func (l *logger) Warnf(format string, v ...any) {
	tflog.Warn(l.ctx, fmt.Sprintf(format, v...))
}

func (l *logger) Errorf(format string, v ...any) {
	// Deliberately logged at warn level: the library reports recoverable
	// conditions (an ack with no registered callback, an event with no
	// handler) through Errorf, and those are normal for this client since it
	// only subscribes to the events it needs.
	tflog.Warn(l.ctx, fmt.Sprintf(format, v...))
}
