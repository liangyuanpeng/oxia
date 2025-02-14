// Copyright 2023 StreamNative, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package model

import (
	"oxia/proto"
)

type PutCall struct {
	Key               string
	Value             []byte
	ExpectedVersionId *int64
	SessionId         *int64
	ClientIdentity    *string
	Callback          func(*proto.PutResponse, error)
}

type DeleteCall struct {
	Key               string
	ExpectedVersionId *int64
	Callback          func(*proto.DeleteResponse, error)
}

type DeleteRangeCall struct {
	MinKeyInclusive string
	MaxKeyExclusive string
	Callback        func(*proto.DeleteRangeResponse, error)
}

type GetCall struct {
	Key      string
	Callback func(*proto.GetResponse, error)
}

func (r PutCall) ToProto() *proto.PutRequest {
	return &proto.PutRequest{
		Key:               r.Key,
		Value:             r.Value,
		ExpectedVersionId: r.ExpectedVersionId,
		SessionId:         r.SessionId,
		ClientIdentity:    r.ClientIdentity,
	}
}

func (r DeleteCall) ToProto() *proto.DeleteRequest {
	return &proto.DeleteRequest{
		Key:               r.Key,
		ExpectedVersionId: r.ExpectedVersionId,
	}
}

func (r DeleteRangeCall) ToProto() *proto.DeleteRangeRequest {
	return &proto.DeleteRangeRequest{
		StartInclusive: r.MinKeyInclusive,
		EndExclusive:   r.MaxKeyExclusive,
	}
}

func (r GetCall) ToProto() *proto.GetRequest {
	return &proto.GetRequest{
		Key:          r.Key,
		IncludeValue: true,
	}
}

func Convert[CALL any, PROTO any](calls []CALL, toProto func(CALL) PROTO) []PROTO {
	protos := make([]PROTO, len(calls))
	for i, call := range calls {
		protos[i] = toProto(call)
	}
	return protos
}
