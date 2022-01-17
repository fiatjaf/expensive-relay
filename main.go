package main

import (
	"fmt"

	"github.com/fiatjaf/relayer"
	"github.com/jmoiron/sqlx"
	"github.com/jmoiron/sqlx/reflectx"
	"github.com/kelseyhightower/envconfig"
	"github.com/lnbits/relampago"
	relampago_connect "github.com/lnbits/relampago/connect"
)

type ExpensiveRelay struct {
	Domain           string `envconfig:"DOMAIN"`
	PostgresDatabase string `envconfig:"POSTGRESQL_DATABASE"`

	SparkoURL       string `envconfig:"SPARKO_URL"`
	SparkoToken     string `envconfig:"SPARKO_TOKEN"`
	LNDHost         string `envconfig:"LND_HOST"`
	LNDCertPath     string `envconfig:"LND_CERT_PATH"`
	LNDMacaroonPath string `envconfig:"LND_MACAROON_PATH"`

	db *sqlx.DB
	ln relampago.Wallet
}

func (relay *ExpensiveRelay) Name() string {
	return "ExpensiveRelay"
}

func (relay *ExpensiveRelay) Init() error {
	err := envconfig.Process("", relay)
	if err != nil {
		return fmt.Errorf("couldn't process envconfig: %w", err)
	}

	if db, err := initDB(relay.PostgresDatabase); err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	} else {
		db.Mapper = reflectx.NewMapperFunc("json", sqlx.NameMapper)
		relay.db = db
	}

	// lightning
	relay.ln, err = relampago_connect.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to lightning backend: %w", err)
	}

	// getting notified of invoice payments
	stream, err := relay.ln.PaidInvoicesStream()
	if err != nil {
		return fmt.Errorf("failed to listen for incoming payments: %w", err)
	}
	go func() {
		for status := range stream {
			handlePaidInvoice(status)
		}
	}()

	// endpoints
	relayer.Router.Path("/").HandlerFunc(handleWebpage)
	relayer.Router.Path("/.well-known/lnurlp/{pubkey}").HandlerFunc(handleLnurlRegister)

	// cleanup events
	go cleanupRoutine()

	return nil
}

var relay = ExpensiveRelay{}

func main() {
	relayer.Start(&relay)
}
