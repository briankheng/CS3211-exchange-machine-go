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
	in  input
	mtx chan struct{}
}

var daemonCh = make(chan inputWithLock)

func daemon() {
	nameToInstrumentCh := make(map[string]chan inputWithLock)
	idToInstrumentCh := make(map[uint32]chan inputWithLock)

	for {
		select {
		case data := <-daemonCh:
			switch data.in.orderType {
			case inputCancel:
				ch, ok := idToInstrumentCh[data.in.orderId]
				if !ok {
					outputOrderDeleted(data.in, false, GetCurrentTimestamp())
					continue
				}

				ch <- data
			default:
				ch, ok := nameToInstrumentCh[data.in.instrument]
				if !ok {
					ch = make(chan inputWithLock, 100000)
					nameToInstrumentCh[data.in.instrument] = ch
					go instrument(ch)
				}
				idToInstrumentCh[data.in.orderId] = ch

				ch <- data
			}
		}
	}
}

func instrument(ch chan inputWithLock) {
	orderBuy := &OrderBookBuy{}
	heap.Init(orderBuy)
	orderSell := &OrderBookSell{}
	heap.Init(orderSell)

	for {
		data := <-ch
		switch data.in.orderType {
		case inputBuy:
			for data.in.count > 0 {
				if len(*orderSell) == 0 {
					break
				}

				if (*orderSell)[0].price > data.in.price {
					break
				}

				if (*orderSell)[0].count > data.in.count {
					outputOrderExecuted((*orderSell)[0].orderId, data.in.orderId, (*orderSell)[0].execId, (*orderSell)[0].price, data.in.count, GetCurrentTimestamp())
					(*orderSell)[0].execId++
					(*orderSell)[0].count -= data.in.count
					data.in.count = 0
				} else {
					outputOrderExecuted((*orderSell)[0].orderId, data.in.orderId, (*orderSell)[0].execId, (*orderSell)[0].price, (*orderSell)[0].count, GetCurrentTimestamp())
					data.in.count -= (*orderSell)[0].count
					heap.Pop(orderSell)
				}
			}
			if data.in.count > 0 {
				heap.Push(orderBuy, orderBookType{data.in.orderId, 1, data.in.price, data.in.count, GetCurrentTimestamp()})
				outputOrderAdded(data.in, GetCurrentTimestamp())
			}
		case inputSell:
			for data.in.count > 0 {
				if len(*orderBuy) == 0 {
					break
				}

				if (*orderBuy)[0].price < data.in.price {
					break
				}

				if (*orderBuy)[0].count > data.in.count {
					outputOrderExecuted((*orderBuy)[0].orderId, data.in.orderId, (*orderBuy)[0].execId, (*orderBuy)[0].price, data.in.count, GetCurrentTimestamp())
					(*orderBuy)[0].execId++
					(*orderBuy)[0].count -= data.in.count
					data.in.count = 0
				} else {
					outputOrderExecuted((*orderBuy)[0].orderId, data.in.orderId, (*orderBuy)[0].execId, (*orderBuy)[0].price, (*orderBuy)[0].count, GetCurrentTimestamp())
					data.in.count -= (*orderBuy)[0].count
					heap.Pop(orderBuy)
				}
			}
			if data.in.count > 0 {
				heap.Push(orderSell, orderBookType{data.in.orderId, 1, data.in.price, data.in.count, GetCurrentTimestamp()})
				outputOrderAdded(data.in, GetCurrentTimestamp())
			}
		case inputCancel:
			flag := false

			for i := 0; i < len(*orderBuy); i++ {
				if (*orderBuy)[i].orderId == data.in.orderId {
					heap.Remove(orderBuy, i)
					outputOrderDeleted(data.in, true, GetCurrentTimestamp())
					flag = true
					break
				}
			}
			for i := 0; i < len(*orderSell); i++ {
				if (*orderSell)[i].orderId == data.in.orderId {
					heap.Remove(orderSell, i)
					outputOrderDeleted(data.in, true, GetCurrentTimestamp())
					flag = true
					break
				}
			}

			if !flag {
				outputOrderDeleted(data.in, false, GetCurrentTimestamp())
			}
		}
		<-data.mtx
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
	mtx := make(chan struct{}, 1)
	for {
		in, err := readInput(conn)
		if err != nil {
			if err != io.EOF {
				_, _ = fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
			}
			return
		}
		mtx <- struct{}{}
		switch in.orderType {
		case inputCancel:
			fmt.Fprintf(os.Stderr, "Got cancel ID: %v\n", in.orderId)
			daemonCh <- inputWithLock{in, mtx}
		default:
			fmt.Fprintf(os.Stderr, "Got order: %c %v x %v @ %v ID: %v\n",
				in.orderType, in.instrument, in.count, in.price, in.orderId)
			daemonCh <- inputWithLock{in, mtx}
		}
	}
}

func GetCurrentTimestamp() int64 {
	return time.Now().UnixNano()
}
