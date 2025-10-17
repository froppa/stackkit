// Package fxeventlog provides a minimal, cleaner Fx event logger backed by zap.
//
// It reduces noise during application boot while preserving important events
// and errors. Intended for use with fx.WithLogger in services.
package fxeventlog

import (
	"strings"
	"time"

	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// MinimalZap is a minimalistic fxevent.Logger.
// It logs important lifecycle events and errors, and keeps boot output tidy.
type MinimalZap struct {
	L   *zap.Logger
	Lvl zapcore.Level // default info

	O Options

	// counters for summaries
	nProvided   int
	nDecorated  int
	nSupplied   int
	nInvoked    int
	startCount  int
	startErrs   int
	startDurSum time.Duration
	stopCount   int
	stopErrs    int
	stopDurSum  time.Duration
}

// Options controls verbosity and summaries for MinimalZap.
type Options struct {
	// Show per-constructor provide events. Errors are always logged.
	ShowProvide bool
	// Show per-decorator events. Errors are always logged.
	ShowDecorate bool
	// Show per-invoke events. Errors are always logged.
	ShowInvoke bool
	// Show lifecycle hook execution logs (OnStart/OnStop). Errors are always logged.
	ShowLifecycle bool
	// Show supplied values; usually noisy. Errors are always logged.
	ShowSupplied bool
	// Emit a compact startup/shutdown summary with counters and durations.
	Summaries bool
}

// DefaultOptions keeps boot logs tidy but informative.
var DefaultOptions = Options{
	ShowProvide:   false,
	ShowDecorate:  false,
	ShowInvoke:    false,
	ShowLifecycle: false,
	ShowSupplied:  false,
	Summaries:     true,
}

// NewMinimal constructs a MinimalZap with sensible defaults.
func NewMinimal(l *zap.Logger) *MinimalZap { return NewWithOptions(l, DefaultOptions) }

// NewWithOptions constructs a MinimalZap with the supplied options.
func NewWithOptions(l *zap.Logger, o Options) *MinimalZap {
	return &MinimalZap{L: l, Lvl: zapcore.InfoLevel, O: o}
}

var _ fxevent.Logger = (*MinimalZap)(nil)

func (m *MinimalZap) log(msg string, fields ...zap.Field) {
	lvl := m.Lvl
	m.L.Log(lvl, msg, fields...)
}

func (m *MinimalZap) logErr(msg string, fields ...zap.Field) {
	m.L.Error(msg, fields...)
}

// LogEvent implements fxevent.Logger.
func (m *MinimalZap) LogEvent(e fxevent.Event) {
	switch ev := e.(type) {
	case *fxevent.Supplied:
		if ev.Err != nil {
			m.logErr("fx.apply_options_error", zap.Error(ev.Err), moduleField(ev.ModuleName))
			return
		}
		m.nSupplied++
		if m.O.ShowSupplied {
			m.log("fx.supplied", moduleField(ev.ModuleName), zap.String("type", ev.TypeName))
		}
	case *fxevent.Provided:
		if ev.Err != nil {
			m.logErr("fx.provide_error", zap.Error(ev.Err), moduleField(ev.ModuleName))
			return
		}
		m.nProvided++
		if m.O.ShowProvide {
			for _, t := range ev.OutputTypeNames {
				m.log("fx.provide", zap.String("constructor", ev.ConstructorName), zap.String("type", t), moduleField(ev.ModuleName))
			}
		}
	case *fxevent.Decorated:
		if ev.Err != nil {
			m.logErr("fx.decorate_error", zap.Error(ev.Err), moduleField(ev.ModuleName))
			return
		}
		m.nDecorated++
		if m.O.ShowDecorate {
			for _, t := range ev.OutputTypeNames {
				m.log("fx.decorate", zap.String("decorator", ev.DecoratorName), zap.String("type", t), moduleField(ev.ModuleName))
			}
		}
	case *fxevent.Invoking:
		if m.O.ShowInvoke {
			m.log("fx.invoke", zap.String("func", ev.FunctionName), moduleField(ev.ModuleName))
		}
	case *fxevent.Invoked:
		m.nInvoked++
		if ev.Err != nil {
			m.logErr("fx.invoke_error", zap.Error(ev.Err), zap.String("func", ev.FunctionName), moduleField(ev.ModuleName))
		} else if m.O.ShowInvoke {
			m.log("fx.invoked", zap.String("func", ev.FunctionName), moduleField(ev.ModuleName))
		}
	case *fxevent.OnStartExecuting:
		if m.O.ShowLifecycle {
			m.log("fx.onstart", zap.String("callee", ev.FunctionName))
		}
	case *fxevent.OnStartExecuted:
		m.startCount++
		m.startDurSum += ev.Runtime
		if ev.Err != nil {
			m.startErrs++
			m.logErr("fx.onstart_error", zap.Error(ev.Err), zap.String("callee", ev.FunctionName), zap.String("runtime", ev.Runtime.String()))
		} else if m.O.ShowLifecycle {
			m.log("fx.onstart_ok", zap.String("callee", ev.FunctionName), zap.String("runtime", ev.Runtime.String()))
		}
	case *fxevent.OnStopExecuting:
		if m.O.ShowLifecycle {
			m.log("fx.onstop", zap.String("callee", ev.FunctionName))
		}
	case *fxevent.OnStopExecuted:
		m.stopCount++
		m.stopDurSum += ev.Runtime
		if ev.Err != nil {
			m.stopErrs++
			m.logErr("fx.onstop_error", zap.Error(ev.Err), zap.String("callee", ev.FunctionName), zap.String("runtime", ev.Runtime.String()))
		} else if m.O.ShowLifecycle {
			m.log("fx.onstop_ok", zap.String("callee", ev.FunctionName), zap.String("runtime", ev.Runtime.String()))
		}
	case *fxevent.Started:
		if ev.Err != nil {
			m.logErr("fx.start_error", zap.Error(ev.Err))
		} else {
			m.log("fx.started")
			if m.O.Summaries {
				m.log("fx.startup_summary",
					zap.Int("provided", m.nProvided),
					zap.Int("decorated", m.nDecorated),
					zap.Int("supplied", m.nSupplied),
					zap.Int("invoked", m.nInvoked),
					zap.Int("hooks", m.startCount),
					zap.Int("hook_errors", m.startErrs),
					zap.String("hook_runtime_total", m.startDurSum.String()),
				)
			}
		}
	case *fxevent.Stopping:
		m.log("fx.stopping", zap.String("signal", strings.ToUpper(ev.Signal.String())))
	case *fxevent.Stopped:
		if ev.Err != nil {
			m.logErr("fx.stop_error", zap.Error(ev.Err))
		} else {
			m.log("fx.stopped")
			if m.O.Summaries {
				m.log("fx.shutdown_summary",
					zap.Int("hooks", m.stopCount),
					zap.Int("hook_errors", m.stopErrs),
					zap.String("hook_runtime_total", m.stopDurSum.String()),
				)
			}
		}
	case *fxevent.RollingBack:
		m.logErr("fx.start_failed_rollback", zap.Error(ev.StartErr))
	case *fxevent.RolledBack:
		if ev.Err != nil {
			m.logErr("fx.rollback_error", zap.Error(ev.Err))
		}
	case *fxevent.LoggerInitialized:
		if ev.Err != nil {
			m.logErr("fx.logger_init_error", zap.Error(ev.Err))
		} else {
			m.log("fx.logger_initialized", zap.String("func", ev.ConstructorName))
		}
	}
}

func moduleField(name string) zap.Field {
	if len(name) == 0 {
		return zap.Skip()
	}
	return zap.String("module", name)
}
