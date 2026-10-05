package main

import (
	"context"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	profileingestion "professional-information-repo/backend/profile-ingestion"
	"professional-information-repo/internal/netlifyproxy"
)

var ingestionHandler = profileingestion.NewHandler()

func main() { lambda.Start(handler) }
func handler(ctx context.Context, event events.APIGatewayProxyRequest) (*events.APIGatewayProxyResponse, error) {
	return netlifyproxy.Handle(ingestionHandler, "/api/profile/ingest", ctx, event)
}
