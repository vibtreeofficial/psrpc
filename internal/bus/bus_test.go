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

package bus_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/vibtreeofficial/psrpc/internal"
	"github.com/vibtreeofficial/psrpc/internal/bus"
	"github.com/vibtreeofficial/psrpc/internal/bus/bustest"
	"github.com/vibtreeofficial/psrpc/pkg/rand"
)

const defaultClientTimeout = time.Second * 3

func busTestChannel(channel string) bus.Channel {
	return bus.Channel{
		Legacy: channel,
		Server: channel,
	}
}

func TestMessageBus(t *testing.T) {
	bustest.TestAll(t, func(t *testing.T, bus func(t testing.TB) bus.MessageBus) {
		b := bus(t)
		t.Run("testSubscribe", func(t *testing.T) { testSubscribe(t, b) })
		t.Run("testSubscribeQueue", func(t *testing.T) { testSubscribeQueue(t, b) })
		t.Run("testSubscribeClose", func(t *testing.T) { testSubscribeClose(t, b) })
	})
}

func testSubscribe(t *testing.T, b bus.MessageBus) {
	ctx := context.Background()

	channel := rand.NewString()
	subA, err := bus.Subscribe[*internal.Request](ctx, b, busTestChannel(channel), bus.DefaultChannelSize)
	require.NoError(t, err)
	subB, err := bus.Subscribe[*internal.Request](ctx, b, busTestChannel(channel), bus.DefaultChannelSize)
	require.NoError(t, err)
	time.Sleep(time.Millisecond * 100)

	require.NoError(t, b.Publish(ctx, busTestChannel(channel), &internal.Request{
		RequestId: "1",
	}))

	msgA := <-subA.Channel()
	msgB := <-subB.Channel()
	require.NotNil(t, msgA)
	require.NotNil(t, msgB)
	require.Equal(t, "1", msgA.RequestId)
	require.Equal(t, "1", msgB.RequestId)
}

func testSubscribeQueue(t *testing.T, b bus.MessageBus) {
	ctx := context.Background()

	channel := rand.NewString()
	subA, err := bus.SubscribeQueue[*internal.Request](ctx, b, busTestChannel(channel), bus.DefaultChannelSize)
	require.NoError(t, err)
	subB, err := bus.SubscribeQueue[*internal.Request](ctx, b, busTestChannel(channel), bus.DefaultChannelSize)
	require.NoError(t, err)
	time.Sleep(time.Millisecond * 100)

	require.NoError(t, b.Publish(ctx, busTestChannel(channel), &internal.Request{
		RequestId: "2",
	}))

	received := 0
	select {
	case m := <-subA.Channel():
		if m != nil {
			received++
		}
	case <-time.After(defaultClientTimeout):
		// continue
	}

	select {
	case m := <-subB.Channel():
		if m != nil {
			received++
		}
	case <-time.After(defaultClientTimeout):
		// continue
	}

	require.Equal(t, 1, received)
}

func testSubscribeClose(t *testing.T, b bus.MessageBus) {
	ctx := context.Background()

	channel := rand.NewString()
	sub, err := bus.Subscribe[*internal.Request](ctx, b, busTestChannel(channel), bus.DefaultChannelSize)
	require.NoError(t, err)

	require.NoError(t, sub.Close())
	time.Sleep(time.Millisecond * 100)

	select {
	case _, ok := <-sub.Channel():
		require.False(t, ok)
	default:
		require.FailNow(t, "closed subscription channel should not block")
	}
}
