package main

import (
	"context"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	qualificationgaps "professional-information-repo/backend/qualification-gaps"
	"professional-information-repo/internal/netlifyproxy"
)

var gapHandler = qualificationgaps.NewHandler()

func main() { lambda.Start(handler) }
func handler(ctx context.Context, event events.APIGatewayProxyRequest) (*events.APIGatewayProxyResponse, error) {
	return netlifyproxy.Handle(gapHandler, "/api/qualification-gaps", ctx, event)
}
