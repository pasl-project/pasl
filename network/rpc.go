package network

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"net"
	"net/http"

	"github.com/creachadair/jrpc2"
	"github.com/creachadair/jrpc2/channel"
	"github.com/creachadair/jrpc2/handler"
	"github.com/creachadair/jrpc2/server"
	"github.com/modern-go/concurrent"
)

type httpFrame struct {
	r       *bufio.Reader
	w       io.WriteCloser
	headers *http.Header
}

func (this *httpFrame) Send(msg []byte) error {
	resp := http.Response{
		StatusCode:    http.StatusOK,
		Body:          ioutil.NopCloser(bytes.NewReader(msg)),
		ContentLength: int64(len(msg)),
		Header:        *this.headers,
		ProtoMajor:    1,
		ProtoMinor:    1,
	}
	result := resp.Write(this.w)
	this.w.Close()
	return result
}

func (this *httpFrame) Recv() ([]byte, error) {
	request, err := http.ReadRequest(this.r)
	if err != nil {
		return nil, err
	}
	return ioutil.ReadAll(request.Body)
}

func (this *httpFrame) Close() error {
	return this.w.Close()
}

func WithRpcServer(hostPort string, handlers map[string]interface{}, callback func() error) error {
	listener, err := net.Listen("tcp", hostPort)
	if err != nil {
		return err
	}

	rpcServer := concurrent.NewUnboundedExecutor()
	rpcServer.Go(func(ctx context.Context) {

		assigner := handler.Map{}
		for each := range handlers {
			assigner[each] = handler.New(handlers[each])
		}

		headers := http.Header{}
		headers.Add("Access-Control-Allow-Origin", "*")
		headers.Add("Content-Type", "application/json")

		server.Loop(listener, server.Static(assigner), &server.LoopOptions{
			Framing: func(r io.Reader, w io.WriteCloser) channel.Channel {
				return &httpFrame{
					r:       bufio.NewReader(r),
					w:       w,
					headers: &headers,
				}
			},
			ServerOptions: &jrpc2.ServerOptions{
				Concurrency:    10,
				AllowV1:        true,
				DisableBuiltin: true,
			},
		})
	})
	defer rpcServer.StopAndWaitForever()
	defer listener.Close()

	return callback()
}
