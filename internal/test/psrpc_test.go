// Copyright 2023 Vibtree, Inc.
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

package test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibtreeofficial/psrpc"
	"github.com/vibtreeofficial/psrpc/internal"
	"github.com/vibtreeofficial/psrpc/internal/bus"
	"github.com/vibtreeofficial/psrpc/internal/bus/bustest"
	"github.com/vibtreeofficial/psrpc/pkg/client"
	"github.com/vibtreeofficial/psrpc/pkg/info"
	"github.com/vibtreeofficial/psrpc/pkg/rand"
	"github.com/vibtreeofficial/psrpc/pkg/server"
)

func TestRPC(t *testing.T) {
	bustest.TestAll(t, func(t *testing.T, bus func(t testing.TB) bus.MessageBus) {
		t.Run("RPC", func(t *testing.T) {
			testRPC(t, bus)
		})
		t.Run("Stream", func(t *testing.T) {
			testStream(t, bus)
		})
	})
}

func testRPC(t *testing.T, bus func(t testing.TB) bus.MessageBus) {
	serviceName := "test"

	serverA := server.NewRPCServer(&info.ServiceDefinition{
		Name: serviceName,
		ID:   rand.NewString(),
	}, bus(t))
	serverB := server.NewRPCServer(&info.ServiceDefinition{
		Name: serviceName,
		ID:   rand.NewString(),
	}, bus(t))
	serverC := server.NewRPCServer(&info.ServiceDefinition{
		Name: serviceName,
		ID:   rand.NewString(),
	}, bus(t))

	t.Cleanup(func() {
		serverA.Close(true)
		serverB.Close(true)
		serverC.Close(true)
	})

	c, err := client.NewRPCClient(&info.ServiceDefinition{
		Name: serviceName,
		ID:   rand.NewString(),
	}, bus(t))
	require.NoError(t, err)

	retErr := psrpc.NewErrorf(psrpc.Internal, "foo")

	counter := 0
	errCount := 0
	rpc := "add_one"
	multiRpc := "add_one_multi"
	addOne := func(ctx context.Context, req *internal.Request) (*internal.Response, error) {
		counter++
		return &internal.Response{RequestId: req.RequestId}, nil
	}
	returnError := func(ctx context.Context, req *internal.Request) (*internal.Response, error) {
		return nil, retErr
	}

	serverA.RegisterMethod(rpc, false, false, true, false)
	serverB.RegisterMethod(rpc, false, false, true, false)
	c.RegisterMethod(rpc, false, false, true, false)

	err = server.RegisterHandler[*internal.Request, *internal.Response](serverA, rpc, nil, addOne, nil)
	require.NoError(t, err)
	err = server.RegisterHandler[*internal.Request, *internal.Response](serverB, rpc, nil, addOne, nil)
	require.NoError(t, err)
	time.Sleep(time.Second)

	ctx := context.Background()
	requestID := rand.NewRequestID()
	res, err := client.RequestSingle[*internal.Response](
		ctx, c, rpc, nil, &internal.Request{RequestId: requestID},
	)

	require.NoError(t, err)
	require.Equal(t, 1, counter)
	require.Equal(t, res.RequestId, requestID)

	serverA.RegisterMethod(multiRpc, false, true, false, false)
	serverB.RegisterMethod(multiRpc, false, true, false, false)
	serverC.RegisterMethod(multiRpc, false, true, false, false)
	c.RegisterMethod(multiRpc, false, true, false, false)

	err = server.RegisterHandler[*internal.Request, *internal.Response](serverA, multiRpc, nil, addOne, nil)
	require.NoError(t, err)
	err = server.RegisterHandler[*internal.Request, *internal.Response](serverB, multiRpc, nil, addOne, nil)
	require.NoError(t, err)
	err = server.RegisterHandler[*internal.Request, *internal.Response](serverC, multiRpc, nil, returnError, nil)
	require.NoError(t, err)
	time.Sleep(time.Second)

	requestID = rand.NewRequestID()
	resChan, err := client.RequestMulti[*internal.Response](
		ctx, c, multiRpc, nil, &internal.Request{RequestId: requestID},
	)
	require.NoError(t, err)

	for i := 0; i < 4; i++ {
		select {
		case res := <-resChan:
			if res == nil {
				require.Equal(t, 3, counter)
				require.Equal(t, 1, errCount)
				return
			}
			if res.Err != nil {
				errCount++
				require.Equal(t, retErr, res.Err)
			} else {
				require.Equal(t, res.Result.RequestId, requestID)
			}
		case <-time.After(psrpc.DefaultClientTimeout + time.Second):
			t.Fatal("response missing")
		}
	}
}

func testStream(t *testing.T, bus func(t testing.TB) bus.MessageBus) {
	serviceName := "test_stream"

	serverA := server.NewRPCServer(&info.ServiceDefinition{
		Name: serviceName,
		ID:   rand.NewString(),
	}, bus(t))

	t.Cleanup(func() {
		serverA.Close(true)
	})

	c, err := client.NewRPCClientWithStreams(&info.ServiceDefinition{
		Name: serviceName,
		ID:   rand.NewString(),
	}, bus(t))
	require.NoError(t, err)

	serverClose := make(chan struct{})
	rpc := "ping_pong"
	handlePing := func(stream psrpc.ServerStream[*internal.Response, *internal.Response]) error {
		defer close(serverClose)

		for ping := range stream.Channel() {
			pong := &internal.Response{
				SentAt: ping.SentAt,
				Code:   "PONG",
			}
			err := stream.Send(pong)
			require.NoError(t, err)
		}
		return nil
	}

	serverA.RegisterMethod(rpc, false, false, true, false)
	c.RegisterMethod(rpc, false, false, true, false)

	err = server.RegisterStreamHandler[*internal.Response, *internal.Response](serverA, rpc, nil, handlePing, nil)
	require.NoError(t, err)
	time.Sleep(time.Second)

	ctx := context.Background()
	stream, err := client.OpenStream[*internal.Response, *internal.Response](
		ctx, c, rpc, nil,
	)
	require.NoError(t, err)

	for i := 0; i < 3; i++ {
		err = stream.Send(&internal.Response{
			Code: "PING",
		})
		require.NoError(t, err)

		select {
		case pong := <-stream.Channel():
			require.Equal(t, "PONG", pong.Code)
		case <-time.After(psrpc.DefaultClientTimeout):
			t.Fatal("no pong received")
		}
	}

	assert.NoError(t, stream.Close(nil))

	select {
	case <-serverClose:
	case <-time.After(psrpc.DefaultClientTimeout):
		t.Fatal("server did not close")
	}
}
