package main

type orderBookType struct {
	orderId   uint32
	execId    uint32
	price     uint32
	count     uint32
	timestamp int64
}

type OrderBookBuy []orderBookType

func (o OrderBookBuy) Len() int { return len(o) }
func (o OrderBookBuy) Less(i, j int) bool {
	if o[i].price == o[j].price {
		return o[i].timestamp < o[j].timestamp
	}
	return o[i].price > o[j].price
}
func (o OrderBookBuy) Swap(i, j int) { o[i], o[j] = o[j], o[i] }

func (o *OrderBookBuy) Push(x interface{}) {
	*o = append(*o, x.(orderBookType))
}

func (o *OrderBookBuy) Pop() interface{} {
	old := *o
	n := len(old)
	x := old[n-1]
	*o = old[0 : n-1]
	return x
}

type OrderBookSell []orderBookType

func (o OrderBookSell) Len() int { return len(o) }
func (o OrderBookSell) Less(i, j int) bool {
	if o[i].price == o[j].price {
		return o[i].timestamp < o[j].timestamp
	}
	return o[i].price < o[j].price
}
func (o OrderBookSell) Swap(i, j int) { o[i], o[j] = o[j], o[i] }

func (o *OrderBookSell) Push(x interface{}) {
	*o = append(*o, x.(orderBookType))
}

func (o *OrderBookSell) Pop() interface{} {
	old := *o
	n := len(old)
	x := old[n-1]
	*o = old[0 : n-1]
	return x
}
