package util

import (
	"log/slog"
	"sync/atomic"
	"time"
)

var slogMeasureID = &atomic.Int64{}

func SLogMeasure(msg string, args ...any) func(args ...any) {
	var (
		then          = time.Now()
		measurementID = slogMeasureID.Add(1)
		allArgs       = append(args, slog.Int64("measurement_id", measurementID))
	)

	slog.Info(msg, append(allArgs, slog.String("state", "enter"))...)

	return func(args ...any) {
		exitArgs := append(allArgs, slog.Duration("elapsed", time.Since(then)), slog.String("state", "exit"))
		exitArgs = append(exitArgs, args...)

		slog.Info(msg, exitArgs...)
	}
}
