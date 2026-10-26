package netlifyproxy

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"

	"github.com/aws/aws-lambda-go/events"
)

// Handle adapts a Netlify (API Gateway v1) event to an http.Handler. The path
// argument is the route the function serves; callers that need the event's
// original path can pass an empty string. Query parameters and multi-value
// response headers are preserved, including multiple Set-Cookie headers.
func Handle(handler http.Handler, path string, ctx context.Context, event events.APIGatewayProxyRequest) (*events.APIGatewayProxyResponse, error) {
	if path == "" {
		path = event.Path
	}
	if path == "" {
		path = "/"
	}
	if query := queryString(event); query != "" {
		path += "?" + query
	}
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
	multiHeaders := make(map[string][]string, len(result.Header))
	for name, values := range result.Header {
		if len(values) == 0 {
			continue
		}
		headers[name] = values[0]
		multiHeaders[name] = append([]string(nil), values...)
	}
	return &events.APIGatewayProxyResponse{
		StatusCode:        result.StatusCode,
		Headers:           headers,
		MultiValueHeaders: multiHeaders,
		Body:              string(responseBody),
	}, nil
}

func queryString(event events.APIGatewayProxyRequest) string {
	values := url.Values{}
	for name, value := range event.QueryStringParameters {
		values.Set(name, value)
	}
	for name, list := range event.MultiValueQueryStringParameters {
		values.Del(name)
		for _, value := range list {
			values.Add(name, value)
		}
	}
	return values.Encode()
}
