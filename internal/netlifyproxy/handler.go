package netlifyproxy

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/aws/aws-lambda-go/events"
)

func Handle(handler http.Handler, path string, ctx context.Context, event events.APIGatewayProxyRequest) (*events.APIGatewayProxyResponse, error) {
	body := []byte(event.Body)
	if event.IsBase64Encoded {
		decoded, err := base64.StdEncoding.DecodeString(event.Body)
		if err != nil {
			return nil, err
		}
		body = decoded
	}
	request := httptest.NewRequest(event.HTTPMethod, path, bytes.NewReader(body))
	request = request.WithContext(ctx)
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
	handler.ServeHTTP(response, request)
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
