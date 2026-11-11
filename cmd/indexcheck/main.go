package main

import (
	"context"
	"fmt"
	"time"

	"marketmind-ai/internal/marketdata"
)

func main() {
	client := marketdata.NewYahooClient()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	for _, index := range []struct {
		name   string
		symbol string
	}{
		{"NIFTY 50", "^NSEI"},
		{"SENSEX", "^BSESN"},
	} {
		fmt.Printf("\n=== %s (%s) ===\n", index.name, index.symbol)

		prices, err := client.FetchIndexPrices(
			ctx, index.symbol, "2026-11-02", "2026-11-09",
		)
		if err != nil {
			fmt.Println("ERROR:", err)
			continue
		}

		fmt.Println("Records:", len(prices))
		for _, p := range prices {
			fmt.Printf(
				"%s | O: %.2f H: %.2f L: %.2f C: %.2f | Volume: ",
				p.Date, p.Open, p.High, p.Low, p.Close,
			)
			if p.Volume == nil {
				fmt.Println("unavailable")
			} else {
				fmt.Println(*p.Volume)
			}
		}
	}
}
