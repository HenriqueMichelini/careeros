package main

import (
	"context"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	cvgeneration "professional-information-repo/backend/cv-generation"
	"professional-information-repo/internal/netlifyproxy"
)

var draftHandler = cvgeneration.NewHandler()

func main() { lambda.Start(handler) }
func handler(ctx context.Context, event events.APIGatewayProxyRequest) (*events.APIGatewayProxyResponse, error) {
	return netlifyproxy.Handle(draftHandler, "/api/cv/generate", ctx, event)
}
