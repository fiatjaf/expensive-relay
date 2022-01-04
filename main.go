package main

import (
	"fmt"

	"github.com/fiatjaf/relayer"
	"github.com/jmoiron/sqlx"
	"github.com/jmoiron/sqlx/reflectx"
	"github.com/kelseyhightower/envconfig"
)

type ExpensiveRelay struct {
	PostgresDatabase string `envconfig:"POSTGRESQL_DATABASE"`
	LNbitsURL        string `envconfig:"LNBITS_URL"`
	LNbitsToken      string `envconfig:"LNBITS_TOKEN"`

	DB *sqlx.DB
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
		relay.DB = db
	}

	relayer.Router.Path("/").HandlerFunc(handleWebpage)
	relayer.Router.Path("/.well-known/lnurlp/{pubkey}").HandlerFunc(handleLnurlRegister)
	relayer.Router.Path("/webhook/invoice-paid").HandlerFunc(handleInvoicePaid)

	go cleanupRoutine(relay.DB)

	return nil
}

var relay = ExpensiveRelay{}

func main() {
	relayer.Start(&relay)
}
