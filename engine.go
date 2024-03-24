package main

import "C"
import (
	"container/heap"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"time"
)

type Engine struct{}

type inputWithLock struct {
	in input
	// mtx chan struct{}
}

var daemonCh = make(chan input)

func daemon() {
	nameToInstrumentCh := make(map[string]chan input)
	idToInstrumentCh := make(map[uint32]chan input)

	for {
		select {
		case in := <-daemonCh:
			switch in.orderType {
			case inputCancel:
				ch, ok := idToInstrumentCh[in.orderId]
				if !ok {
					outputOrderDeleted(in, false, GetCurrentTimestamp())
					continue
				}

				ch <- in
			default:
				ch, ok := nameToInstrumentCh[in.instrument]
				if !ok {
					ch = make(chan input, 100000)
					nameToInstrumentCh[in.instrument] = ch
					go instrument(ch)
				}
				idToInstrumentCh[in.orderId] = ch

				ch <- in
			}
		}
	}
}

func instrument(ch chan input) {
	orderBuy := &OrderBookBuy{}
	heap.Init(orderBuy)
	orderSell := &OrderBookSell{}
	heap.Init(orderSell)

	for {
		in := <-ch
		switch in.orderType {
		case inputBuy:
			for in.count > 0 {
				if len(*orderSell) == 0 {
					break
				}

				if (*orderSell)[0].price > in.price {
					break
				}

				if (*orderSell)[0].count > in.count {
					outputOrderExecuted((*orderSell)[0].orderId, in.orderId, (*orderSell)[0].execId, (*orderSell)[0].price, in.count, GetCurrentTimestamp())
					(*orderSell)[0].execId++
					(*orderSell)[0].count -= in.count
					in.count = 0
				} else {
					outputOrderExecuted((*orderSell)[0].orderId, in.orderId, (*orderSell)[0].execId, (*orderSell)[0].price, (*orderSell)[0].count, GetCurrentTimestamp())
					in.count -= (*orderSell)[0].count
					heap.Pop(orderSell)
				}
			}
			if in.count > 0 {
				heap.Push(orderBuy, orderBookType{in.orderId, 1, in.price, in.count, GetCurrentTimestamp()})
				outputOrderAdded(in, GetCurrentTimestamp())
			}
		case inputSell:
			for in.count > 0 {
				if len(*orderBuy) == 0 {
					break
				}

				if (*orderBuy)[0].price < in.price {
					break
				}

				if (*orderBuy)[0].count > in.count {
					outputOrderExecuted((*orderBuy)[0].orderId, in.orderId, (*orderBuy)[0].execId, (*orderBuy)[0].price, in.count, GetCurrentTimestamp())
					(*orderBuy)[0].execId++
					(*orderBuy)[0].count -= in.count
					in.count = 0
				} else {
					outputOrderExecuted((*orderBuy)[0].orderId, in.orderId, (*orderBuy)[0].execId, (*orderBuy)[0].price, (*orderBuy)[0].count, GetCurrentTimestamp())
					in.count -= (*orderBuy)[0].count
					heap.Pop(orderBuy)
				}
			}
			if in.count > 0 {
				heap.Push(orderSell, orderBookType{in.orderId, 1, in.price, in.count, GetCurrentTimestamp()})
				outputOrderAdded(in, GetCurrentTimestamp())
			}
		case inputCancel:
			flag := false

			for i := 0; i < len(*orderBuy); i++ {
				if (*orderBuy)[i].orderId == in.orderId {
					heap.Remove(orderBuy, i)
					outputOrderDeleted(in, true, GetCurrentTimestamp())
					flag = true
					break
				}
			}
			for i := 0; i < len(*orderSell); i++ {
				if (*orderSell)[i].orderId == in.orderId {
					heap.Remove(orderSell, i)
					outputOrderDeleted(in, true, GetCurrentTimestamp())
					flag = true
					break
				}
			}

			if !flag {
				outputOrderDeleted(in, false, GetCurrentTimestamp())
			}
		}
	}
}

func (e *Engine) init() {
	go daemon()
}

func (e *Engine) accept(ctx context.Context, conn net.Conn) {
	go func() {
		<-ctx.Done()
		conn.Close()
	}()
	go handleConn(conn)
}

func handleConn(conn net.Conn) {
	defer conn.Close()
	for {
		in, err := readInput(conn)
		if err != nil {
			if err != io.EOF {
				_, _ = fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
			}
			return
		}
		switch in.orderType {
		case inputCancel:
			fmt.Fprintf(os.Stderr, "Got cancel ID: %v\n", in.orderId)
			daemonCh <- in
		default:
			fmt.Fprintf(os.Stderr, "Got order: %c %v x %v @ %v ID: %v\n",
				in.orderType, in.instrument, in.count, in.price, in.orderId)
			daemonCh <- in
		}
	}
}

func GetCurrentTimestamp() int64 {
	return time.Now().UnixNano()
}
