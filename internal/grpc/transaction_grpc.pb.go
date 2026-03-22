package grpc

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

const (
	TransactionService_CheckTransaction_FullMethodName = "/transaction.TransactionService/CheckTransaction"
	TransactionService_GetTransactions_FullMethodName  = "/transaction.TransactionService/GetTransactions"
	TransactionService_Health_FullMethodName          = "/transaction.TransactionService/Health"
)

// TransactionServiceClient is the client API for TransactionService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type TransactionServiceClient interface {
	// Check transaction by payment code (content)
	CheckTransaction(ctx context.Context, in *CheckTransactionRequest, opts ...grpc.CallOption) (*CheckTransactionResponse, error)
	// Get transactions in date range
	GetTransactions(ctx context.Context, in *GetTransactionsRequest, opts ...grpc.CallOption) (*GetTransactionsResponse, error)
	// Health check
	Health(ctx context.Context, in *HealthRequest, opts ...grpc.CallOption) (*HealthResponse, error)
}

type transactionServiceClient struct {
	cc grpc.ClientConnInterface
}

// NewTransactionServiceClient creates a new TransactionServiceClient
func NewTransactionServiceClient(cc grpc.ClientConnInterface) TransactionServiceClient {
	return &transactionServiceClient{cc}
}

func (c *transactionServiceClient) CheckTransaction(ctx context.Context, in *CheckTransactionRequest, opts ...grpc.CallOption) (*CheckTransactionResponse, error) {
	out := new(CheckTransactionResponse)
	err := c.cc.Invoke(ctx, TransactionService_CheckTransaction_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *transactionServiceClient) GetTransactions(ctx context.Context, in *GetTransactionsRequest, opts ...grpc.CallOption) (*GetTransactionsResponse, error) {
	out := new(GetTransactionsResponse)
	err := c.cc.Invoke(ctx, TransactionService_GetTransactions_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *transactionServiceClient) Health(ctx context.Context, in *HealthRequest, opts ...grpc.CallOption) (*HealthResponse, error) {
	out := new(HealthResponse)
	err := c.cc.Invoke(ctx, TransactionService_Health_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// TransactionServiceServer is the server API for TransactionService service.
// All implementations must embed UnimplementedTransactionServiceServer
// for forward compatibility
type TransactionServiceServer interface {
	// Check transaction by payment code (content)
	CheckTransaction(context.Context, *CheckTransactionRequest) (*CheckTransactionResponse, error)
	// Get transactions in date range
	GetTransactions(context.Context, *GetTransactionsRequest) (*GetTransactionsResponse, error)
	// Health check
	Health(context.Context, *HealthRequest) (*HealthResponse, error)
	mustEmbedUnimplementedTransactionServiceServer()
}

// UnimplementedTransactionServiceServer must be embedded to have forward compatible implementations.
type UnimplementedTransactionServiceServer struct{}

func (UnimplementedTransactionServiceServer) CheckTransaction(context.Context, *CheckTransactionRequest) (*CheckTransactionResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CheckTransaction not implemented")
}
func (UnimplementedTransactionServiceServer) GetTransactions(context.Context, *GetTransactionsRequest) (*GetTransactionsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetTransactions not implemented")
}
func (UnimplementedTransactionServiceServer) Health(context.Context, *HealthRequest) (*HealthResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Health not implemented")
}
func (UnimplementedTransactionServiceServer) mustEmbedUnimplementedTransactionServiceServer() {}

func RegisterTransactionServiceServer(s *grpc.Server, srv TransactionServiceServer) {
	s.RegisterService(&_TransactionService_serviceDesc, srv)
}

var _TransactionService_serviceDesc = grpc.ServiceDesc{
	ServiceName: "transaction.TransactionService",
	HandlerType: (*TransactionServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "CheckTransaction",
			Handler:    _TransactionService_CheckTransaction_Handler,
		},
		{
			MethodName: "GetTransactions",
			Handler:    _TransactionService_GetTransactions_Handler,
		},
		{
			MethodName: "Health",
			Handler:    _TransactionService_Health_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "transaction.proto",
}

func _TransactionService_CheckTransaction_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CheckTransactionRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TransactionServiceServer).CheckTransaction(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: TransactionService_CheckTransaction_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TransactionServiceServer).CheckTransaction(ctx, req.(*CheckTransactionRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _TransactionService_GetTransactions_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetTransactionsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TransactionServiceServer).GetTransactions(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: TransactionService_GetTransactions_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TransactionServiceServer).GetTransactions(ctx, req.(*GetTransactionsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _TransactionService_Health_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(HealthRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TransactionServiceServer).Health(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: TransactionService_Health_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TransactionServiceServer).Health(ctx, req.(*HealthRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// Request/Response types

type CheckTransactionRequest struct {
	PaymentCode   string `protobuf:"bytes,1,opt,name=payment_code,json=paymentCode" json:"payment_code,omitempty"`
	FromTimestamp int64  `protobuf:"varint,2,opt,name=from_timestamp,json=fromTimestamp" json:"from_timestamp,omitempty"`
	ToTimestamp   int64  `protobuf:"varint,3,opt,name=to_timestamp,json=toTimestamp" json:"to_timestamp,omitempty"`
}

func (m *CheckTransactionRequest) Reset()         { *m = CheckTransactionRequest{} }
func (m *CheckTransactionRequest) String() string { return m.String() }
func (*CheckTransactionRequest) ProtoMessage()    {}

type CheckTransactionResponse struct {
	Found             bool   `protobuf:"varint,1,opt,name=found" json:"found,omitempty"`
	TransactionId     string `protobuf:"bytes,2,opt,name=transaction_id,json=transactionId" json:"transaction_id,omitempty"`
	Amount            string `protobuf:"bytes,3,opt,name=amount" json:"amount,omitempty"`
	Currency          string `protobuf:"bytes,4,opt,name=currency" json:"currency,omitempty"`
	Description       string `protobuf:"bytes,5,opt,name=description" json:"description,omitempty"`
	TransactionDate   string `protobuf:"bytes,6,opt,name=transaction_date,json=transactionDate" json:"transaction_date,omitempty"`
	Status            string `protobuf:"bytes,7,opt,name=status" json:"status,omitempty"`
	ErrorMessage      string `protobuf:"bytes,8,opt,name=error_message,json=errorMessage" json:"error_message,omitempty"`
}

func (m *CheckTransactionResponse) Reset()         { *m = CheckTransactionResponse{} }
func (m *CheckTransactionResponse) String() string { return m.String() }
func (*CheckTransactionResponse) ProtoMessage()    {}

type GetTransactionsRequest struct {
	FromTimestamp int64 `protobuf:"varint,1,opt,name=from_timestamp,json=fromTimestamp" json:"from_timestamp,omitempty"`
	ToTimestamp   int64 `protobuf:"varint,2,opt,name=to_timestamp,json=toTimestamp" json:"to_timestamp,omitempty"`
	Limit         int32 `protobuf:"varint,3,opt,name=limit" json:"limit,omitempty"`
}

func (m *GetTransactionsRequest) Reset()         { *m = GetTransactionsRequest{} }
func (m *GetTransactionsRequest) String() string { return m.String() }
func (*GetTransactionsRequest) ProtoMessage()    {}

type TransactionData struct {
	PostingDate        string `protobuf:"bytes,1,opt,name=posting_date,json=postingDate" json:"posting_date,omitempty"`
	TransactionDate    string `protobuf:"bytes,2,opt,name=transaction_date,json=transactionDate" json:"transaction_date,omitempty"`
	AccountNo          string `protobuf:"bytes,3,opt,name=account_no,json=accountNo" json:"account_no,omitempty"`
	CreditAmount       string `protobuf:"bytes,4,opt,name=credit_amount,json=creditAmount" json:"credit_amount,omitempty"`
	DebitAmount        string `protobuf:"bytes,5,opt,name=debit_amount,json=debitAmount" json:"debit_amount,omitempty"`
	Currency           string `protobuf:"bytes,6,opt,name=currency" json:"currency,omitempty"`
	Description        string `protobuf:"bytes,7,opt,name=description" json:"description,omitempty"`
	AddDescription     string `protobuf:"bytes,8,opt,name=add_description,json=addDescription" json:"add_description,omitempty"`
	AvailableBalance   string `protobuf:"bytes,9,opt,name=available_balance,json=availableBalance" json:"available_balance,omitempty"`
	RefNo              string `protobuf:"bytes,10,opt,name=ref_no,json=refNo" json:"ref_no,omitempty"`
	BenAccountName     string `protobuf:"bytes,11,opt,name=ben_account_name,json=benAccountName" json:"ben_account_name,omitempty"`
	BankName           string `protobuf:"bytes,12,opt,name=bank_name,json=bankName" json:"bank_name,omitempty"`
	TransactionType    string `protobuf:"bytes,13,opt,name=transaction_type,json=transactionType" json:"transaction_type,omitempty"`
}

func (m *TransactionData) Reset()         { *m = TransactionData{} }
func (m *TransactionData) String() string { return m.String() }
func (*TransactionData) ProtoMessage()    {}

type GetTransactionsResponse struct {
	Success      bool              `protobuf:"varint,1,opt,name=success" json:"success,omitempty"`
	Count        int32             `protobuf:"varint,2,opt,name=count" json:"count,omitempty"`
	Transactions []*TransactionData `protobuf:"bytes,3,rep,name=transactions" json:"transactions,omitempty"`
}

func (m *GetTransactionsResponse) Reset()         { *m = GetTransactionsResponse{} }
func (m *GetTransactionsResponse) String() string { return m.String() }
func (*GetTransactionsResponse) ProtoMessage()    {}

type HealthRequest struct {
}

func (m *HealthRequest) Reset()         { *m = HealthRequest{} }
func (m *HealthRequest) String() string { return m.String() }
func (*HealthRequest) ProtoMessage()    {}

type HealthResponse struct {
	Healthy bool   `protobuf:"varint,1,opt,name=healthy" json:"healthy,omitempty"`
	Message string `protobuf:"bytes,2,opt,name=message" json:"message,omitempty"`
}

func (m *HealthResponse) Reset()         { *m = HealthResponse{} }
func (m *HealthResponse) String() string { return m.String() }
func (*HealthResponse) ProtoMessage()    {}
