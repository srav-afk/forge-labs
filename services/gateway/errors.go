package gateway

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func selectErrorStatus(err error) (httpStatus int, errType, code string) {
	if errors.Is(err, ErrNoSnapshot) {
		return http.StatusServiceUnavailable, "server_error", "no_snapshot"
	}
	if errors.Is(err, errNoCapacity) {
		return http.StatusServiceUnavailable, "server_error", "no_capacity"
	}
	return http.StatusNotFound, "invalid_request_error", "model_not_found"
}

type openAIErrorBody struct {
	Error openAIError `json:"error"`
}

type openAIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

func writeOpenAIError(c *gin.Context, httpStatus int, message, errType, code string) {
	c.AbortWithStatusJSON(httpStatus, openAIErrorBody{
		Error: openAIError{
			Message: message,
			Type:    errType,
			Code:    code,
		},
	})
}

func mapGRPCError(err error) (httpStatus int, message, errType, code string) {
	st, ok := status.FromError(err)
	if !ok {
		return http.StatusInternalServerError, err.Error(), "server_error", "internal"
	}
	switch st.Code() {
	case codes.NotFound:
		return http.StatusNotFound, st.Message(), "invalid_request_error", "model_not_found"
	case codes.Unavailable:
		return http.StatusServiceUnavailable, st.Message(), "server_error", "unavailable"
	case codes.ResourceExhausted:
		return http.StatusTooManyRequests, st.Message(), "server_error", "rate_limit"
	case codes.DeadlineExceeded:
		return http.StatusGatewayTimeout, st.Message(), "server_error", "timeout"
	case codes.InvalidArgument:
		return http.StatusBadRequest, st.Message(), "invalid_request_error", "invalid_request"
	case codes.Canceled:
		return http.StatusRequestTimeout, st.Message(), "server_error", "canceled"
	default:
		return http.StatusInternalServerError, st.Message(), "server_error", "internal"
	}
}
