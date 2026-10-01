package main

import (
	"context"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	profilereview "professional-information-repo/backend/profile-review"
	"professional-information-repo/internal/netlifyproxy"
)

var reviewHandler = profilereview.NewHandler()

func main() {
	lambda.Start(handler)
}

func handler(ctx context.Context, event events.APIGatewayProxyRequest) (*events.APIGatewayProxyResponse, error) {
	return netlifyproxy.Handle(reviewHandler, "/api/profile/review", ctx, event)
}
