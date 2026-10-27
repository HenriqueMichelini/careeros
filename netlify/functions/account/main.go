package main

import (
	"context"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"net/http"
	"professional-information-repo/internal/accountapi"
	"professional-information-repo/internal/netlifyproxy"
	"strings"
	"sync"
)

var runtimeMu sync.Mutex
var runtime *accountapi.Runtime

func main() { lambda.Start(handler) }
func configured(ctx context.Context) http.Handler {
	runtimeMu.Lock()
	defer runtimeMu.Unlock()
	if runtime == nil {
		runtime, _ = accountapi.FromEnv(ctx)
	}
	if runtime == nil {
		return accountapi.New(nil, nil)
	}
	return runtime.Handler
}
func handler(ctx context.Context, event events.APIGatewayProxyRequest) (*events.APIGatewayProxyResponse, error) {
	// Preserve either Netlify's original path or the direct function path. Both
	// reach the same protected router, including direct identifier substitution.
	path := strings.TrimPrefix(event.Path, "/.netlify/functions/account")
	return netlifyproxy.Handle(configured(ctx), path, ctx, event)
}
