package main

import (
	"context"
	"net/http"

	"github.com/froppa/stackkit/kits/configkit"
	"github.com/froppa/stackkit/kits/fxeventlog"
	"github.com/froppa/stackkit/kits/healthkit"
	"github.com/froppa/stackkit/kits/httpkit"
	"github.com/froppa/stackkit/kits/logkit"
	"github.com/froppa/stackkit/kits/shutdownkit"
	"github.com/froppa/stackkit/kits/telemetry"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"
)

func main() {
	app := fx.New(
		fx.WithLogger(func(log *zap.Logger) fxevent.Logger {
			return fxeventlog.NewMinimal(log)
		}),
		fx.Supply(context.Background()),
		configkit.Module(
			configkit.WithEmbeddedBytes([]byte(`
http:
  addr: ":8080"
health: {}
telemetry:
  disabled: true
`)),
		),
		logkit.Module(),
		httpkit.Module(),
		healthkit.ServerModule(),
		telemetry.Module(),
		shutdownkit.Module(),
		fx.Provide(fx.Annotate(
			func() httpkit.Handler {
				return httpkit.Handler{
					Pattern: "/ping",
					Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						_, _ = w.Write([]byte("pong"))
					}),
				}
			},
			fx.ResultTags(`group:"http.handlers"`),
		)),
	)

	app.Run()
}
