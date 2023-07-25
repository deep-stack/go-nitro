package main

import (
	"fmt"
	"log"
	"math/big"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog"
	"github.com/statechannels/go-nitro/reverseproxy"
	"github.com/urfave/cli/v2"
)

const (
	NITRO_ENDPOINT          = "nitroendpoint"
	PROXY_ADDRESS           = "proxyaddress"
	DESTINATION_URL         = "destinationurl"
	EXPECTED_PAYMENT_AMOUNT = "expectedpaymentamount"
)

func main() {
	var rProxy *reverseproxy.ReversePaymentProxy
	app := &cli.App{
		Name:  "start-reverse-payment-proxy",
		Usage: "Runs an HTTP payment proxy that charges for HTTP requests",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    PROXY_ADDRESS,
				Usage:   "Specifies the TCP address for the proxy to listen on for requests. This should be in the form 'host:port'",
				Value:   "localhost:5511",
				Aliases: []string{"p"},
			},
			&cli.StringFlag{
				Name:    NITRO_ENDPOINT,
				Usage:   "Specifies the endpoint of the Nitro RPC server to connect to. This should be in the form 'host:port/api/v1'",
				Value:   "localhost:4007/api/v1",
				Aliases: []string{"n"},
			},
			&cli.StringFlag{
				Name:    DESTINATION_URL,
				Usage:   "Specifies the destination URL to forward requests to. It should be a fully qualified URL, including the protocol (e.g. http://localhost:8081)",
				Value:   "http://localhost:8081",
				Aliases: []string{"d"},
			},
			&cli.Uint64Flag{
				Name:    EXPECTED_PAYMENT_AMOUNT,
				Usage:   "Specifies the amount of wei that the proxy expects to receive for each request from the voucher",
				Value:   5,
				Aliases: []string{"e"},
			},
		},
		Action: func(c *cli.Context) error {
			proxyEndpoint := c.String(PROXY_ADDRESS)
			nitroEndpoint := c.String(NITRO_ENDPOINT)

			// For now we just log to stdout
			logger := zerolog.New(os.Stdout).Level(zerolog.DebugLevel)

			rProxy = reverseproxy.NewReversePaymentProxy(
				proxyEndpoint,
				nitroEndpoint,
				c.String(DESTINATION_URL),
				big.NewInt(c.Int64(EXPECTED_PAYMENT_AMOUNT)),
				logger)

			return rProxy.Start()
		},
	}
	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
	waitForKillSignal()
	if rProxy != nil {
		err := rProxy.Stop()
		if err != nil {
			log.Fatal(err)
		}
	}
}

// waitForKillSignal blocks until we receive a kill or interrupt signal
func waitForKillSignal() {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigs
	fmt.Printf("Received signal %s, exiting..\n", sig)
}