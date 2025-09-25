package vfs

import (
	"context"

	"github.com/sirupsen/logrus"
)

func Logger(ctx context.Context) *logrus.Entry {
	if logger, ok := ctx.Value(Logger).(*logrus.Logger); ok {
		return logrus.NewEntry(logger)
	}

	if logger, ok := ctx.Value(Logger).(*logrus.Entry); ok {
		return logger
	}

	return logrus.NewEntry(logrus.StandardLogger())
}
