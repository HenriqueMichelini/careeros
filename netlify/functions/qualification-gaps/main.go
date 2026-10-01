package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net/http/httptest"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	qualificationgaps "professional-information-repo/backend/qualification-gaps"
)

var gapHandler = qualificationgaps.NewHandler()

func main() { lambda.Start(handler) }
func handler(_ context.Context, event events.APIGatewayProxyRequest) (*events.APIGatewayProxyResponse, error) {
	body := []byte(event.Body)
	if event.IsBase64Encoded {
		decoded, err := base64.StdEncoding.DecodeString(event.Body)
		if err != nil {
			return nil, err
		}
		body = decoded
	}
	path := event.Path
	if path != "/api/qualification-gaps" {
		path = "/api/qualification-gaps"
	}
	request := httptest.NewRequest(event.HTTPMethod, path, bytes.NewReader(body))
	for name, value := range event.Headers {
		request.Header.Set(name, value)
	}
	for name, values := range event.MultiValueHeaders {
		request.Header.Del(name)
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}
	response := httptest.NewRecorder()
	gapHandler.ServeHTTP(response, request)
	result := response.Result()
	defer result.Body.Close()
	responseBody, err := io.ReadAll(result.Body)
	if err != nil {
		return nil, err
	}
	headers := make(map[string]string, len(result.Header))
	for name, values := range result.Header {
		if len(values) > 0 {
			headers[name] = values[0]
		}
	}
	return &events.APIGatewayProxyResponse{StatusCode: result.StatusCode, Headers: headers, Body: string(responseBody)}, nil
}
