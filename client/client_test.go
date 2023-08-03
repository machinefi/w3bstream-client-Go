package client

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestNewEvent(t *testing.T) {
	require := require.New(t)

	t.Run("newEvent", func(t *testing.T) {
		data := "payload"
		id := "id"
		event := newEvent(
			&Header{
				DeviceID: id,
			},
			[]byte(data),
		)
		require.Equal(data, event.Payload)
		require.Equal(id, event.DeviceID)
		require.Equal("DEFAULT", event.EventType)
		require.Greater(event.Timestamp, int64(0))
	})

	t.Run("newEvent_eventType", func(t *testing.T) {
		data := "payload"
		id := "id"
		eventType := "test"
		event := newEvent(
			&Header{
				DeviceID:  id,
				EventType: eventType,
			},
			[]byte(data),
		)
		require.Equal(data, event.Payload)
		require.Equal(id, event.DeviceID)
		require.Equal(eventType, event.EventType)
		require.Greater(event.Timestamp, int64(0))
	})

	t.Run("newEvent_timestamp", func(t *testing.T) {
		data := "payload"
		id := "id"
		eventType := "test"
		tt := time.Now()
		event := newEvent(
			&Header{
				DeviceID:  id,
				EventType: eventType,
				Timestamp: tt,
			},
			[]byte(data),
		)
		require.Equal(data, event.Payload)
		require.Equal(id, event.DeviceID)
		require.Equal(eventType, event.EventType)
		require.Equal(tt.Unix(), event.Timestamp)
	})
}

func TestPublishEventSync(t *testing.T) {
	require := require.New(t)

	t.Run("success", func(t *testing.T) {
		testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
			require.Equal("POST", req.Method)
			data, err := io.ReadAll(req.Body)
			require.NoError(err)
			v := []*event{}
			err = json.Unmarshal(data, &v)
			require.NoError(err)
			require.Equal(1, len(v))
			require.Equal("payload", v[0].Payload)
			require.Equal("id", v[0].DeviceID)

			res.WriteHeader(200)
			res.Write([]byte("body"))
		}))
		defer func() { testServer.Close() }()

		cli := NewClient(testServer.URL, "key")
		defer cli.Close()
		resp, err := cli.PublishEventSync(
			&Header{
				DeviceID: "id",
			},
			[]byte("payload"),
		)

		require.NoError(err)
		require.Equal(200, resp.StatusCode)
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(err)
		require.Equal("body", string(body))
	})

	t.Run("failed", func(t *testing.T) {
		testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
			require.Equal("POST", req.Method)
			data, err := io.ReadAll(req.Body)
			require.NoError(err)
			v := []*event{}
			err = json.Unmarshal(data, &v)
			require.NoError(err)
			require.Equal(1, len(v))
			require.Equal("payload", v[0].Payload)
			require.Equal("id", v[0].DeviceID)

			res.WriteHeader(500)
			res.Write([]byte("failed"))
		}))
		defer func() { testServer.Close() }()

		cli := NewClient(testServer.URL, "key")
		defer cli.Close()
		resp, err := cli.PublishEventSync(
			&Header{
				DeviceID: "id",
			},
			[]byte("payload"),
		)

		require.NoError(err)
		require.Equal(500, resp.StatusCode)
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(err)
		require.Equal("failed", string(body))
	})

}

func TestPublishEventAsync(t *testing.T) {
	require := require.New(t)
	defer goleak.VerifyNone(t)

	t.Run("success", func(t *testing.T) {
		var (
			counter    uint32 = 0
			eventsSize        = 1000
		)
		testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
			require.Equal("POST", req.Method)
			data, err := io.ReadAll(req.Body)
			require.NoError(err)
			v := []*event{}
			err = json.Unmarshal(data, &v)
			require.NoError(err)
			require.Greater(len(v), 0)
			require.Equal("payload", v[0].Payload)
			require.Equal("id", v[0].DeviceID)

			res.WriteHeader(200)
			res.Write([]byte("body"))
			atomic.AddUint32(&counter, uint32(len(v)))
			log.Println(atomic.LoadUint32(&counter))
		}))
		defer testServer.Close()

		cli := NewClient(testServer.URL, "key")
		defer cli.Close()

		for i := 0; i < eventsSize; i++ {
			err := cli.PublishEvent(
				&Header{
					DeviceID: "id",
				},
				[]byte("payload"),
			)
			require.NoError(err)
		}

		timer1 := time.NewTimer(5 * time.Second)
		for {
			select {
			case <-timer1.C:
				require.Fail("timeout")
			default:
				if atomic.LoadUint32(&counter) == uint32(eventsSize) {
					return
				}
			}
		}
	})

}

func BenchmarkPublishEventAsync(t *testing.B) {
	testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		res.WriteHeader(200)
		res.Write([]byte("body"))
	}))
	cli := NewClient(testServer.URL, "key")

	for i := 0; i < t.N; i++ {
		_ = cli.PublishEvent(
			&Header{
				DeviceID: "id",
			},
			[]byte("payload"),
		)
	}
}
