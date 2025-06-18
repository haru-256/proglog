package log_v1

import (
	"fmt"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ErrOffsetOutOfRange struct {
	Offset uint64
}

// GRPCStatus returns a gRPC status for ErrOffsetOutOfRange.
// It includes a status code of OutOfRange and a message indicating the offset
// is out of range. It also includes a LocalizedMessage detail with a user-friendly
// message.
// GRPCStatusメソッドを持つinterfaceを実装することで、client側で、status.Code関数でerrorからcodeを取得することができる。
func (e ErrOffsetOutOfRange) GRPCStatus() *status.Status {
	st := status.New(codes.OutOfRange, fmt.Sprintf("offset out of range: %d", e.Offset))
	msg := fmt.Sprintf("The requested offset is outside the log's range: %d", e.Offset)

	d := errdetails.LocalizedMessage{
		Locale:  "en-US",
		Message: msg,
	}
	std, err := st.WithDetails(&d)
	if err == nil {
		return std
	}
	return std
}

func (e ErrOffsetOutOfRange) Error() string {
	return e.GRPCStatus().Err().Error()
}
