package main

import (
	"context"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	applicationdraft "professional-information-repo/backend/application-draft"
	"professional-information-repo/internal/netlifyproxy"
)

var draftHandler = applicationdraft.NewHandler()

func main() { lambda.Start(handler) }
func handler(ctx context.Context, event events.APIGatewayProxyRequest) (*events.APIGatewayProxyResponse, error) {
	return netlifyproxy.Handle(draftHandler, "/api/application-draft", ctx, event)
}
