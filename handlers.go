package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/fiatjaf/go-lnurl"
	"github.com/fiatjaf/relayer"
	"github.com/gorilla/mux"
	"github.com/lnbits/relampago"
)

const PRICE_SAT = 500

func handleWebpage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(`
<meta charset=utf-8>
<title>expensive relay</title>

<h1>expensive relay</h1>

<a href="https://github.com/fiatjaf/expensive-relay">https://github.com/fiatjaf/expensive-relay</a>

<p>this is a nostr relay that only accepts events published from keys that pay a registration fee. this is an antispam measure. you can still be banned if you're spamming or doing something bad.</p>

<p>to register your nostr public key, type it below and click the button. or send ` + strconv.Itoa(PRICE_SAT) + ` satoshis to <code>&lt;yourpubkey&gt;@` + relay.Domain + `</code>.</p>

<form>
  <label>
    nostr public key:
    <input name=pubkey />
  </label>
  <button>Get Invoice</button>
</form>
<p id=message></p>
<a id=link><canvas id=qr /></a>
<code id=invoice></code>

<script src="https://cdnjs.cloudflare.com/ajax/libs/qrious/4.0.2/qrious.min.js"></script>
<script>
document.querySelector('form').addEventListener('submit', async ev => {
  ev.preventDefault()
  let res = await (await fetch('/.well-known/lnurlp/' + ev.target.pubkey.value + '?amount=` + strconv.Itoa(PRICE_SAT*1000) + `')).text()
  let { pr, reason } = JSON.parse(res)

  if (pr) {
    invoice.innerHTML = pr
    link.href = 'lightning:' + pr
    new QRious({
      element: qr,
      value: pr.toUpperCase(),
      size: 300
    });
  } else {
    message.innerHTML = reason
  }
})
</script>

<style>
body {
  margin: 10px auto;
  width: 800px;
  max-width: 90%;
}
</style>
    `))
}

func handleLnurlRegister(w http.ResponseWriter, r *http.Request) {
	pubkey := mux.Vars(r)["pubkey"]
	if v, err := hex.DecodeString(pubkey); err != nil || len(v) != 32 {
		json.NewEncoder(w).Encode(lnurl.LNURLResponse{
			Status: "ERROR",
			Reason: "invalid pubkey " + pubkey,
		})
		return
	}

	metadata, _ := json.Marshal([][]string{
		{"text/plain", "registration for pubkey " + pubkey},
	})

	if amount := r.URL.Query().Get("amount"); amount == "" {
		json.NewEncoder(w).Encode(lnurl.LNURLPayParams{
			LNURLResponse: lnurl.LNURLResponse{Status: "OK"},
			Callback: fmt.Sprintf("https://%s/.well-known/lnurlp/%s",
				relay.Domain, pubkey),
			MinSendable:     PRICE_SAT * 1000,
			MaxSendable:     PRICE_SAT * 1000,
			EncodedMetadata: string(metadata),
			Tag:             "payRequest",
		})
	} else {
		msat, err := strconv.Atoi(amount)
		if err != nil || msat != PRICE_SAT*1000 {
			json.NewEncoder(w).Encode(lnurl.ErrorResponse("invalid amount " + amount))
			return
		}

		h := sha256.Sum256(metadata)
		inv, err := relay.ln.CreateInvoice(relampago.InvoiceParams{
			Msatoshi:        int64(msat),
			DescriptionHash: h[:],
		})
		if err != nil {
			json.NewEncoder(w).Encode(
				lnurl.ErrorResponse("failed to create invoice: " + err.Error()))
			return
		}

		var invoice string
		if err := relay.db.Get(&invoice, `
            INSERT INTO registered_users (pubkey, invoice) VALUES ($1, $2)
            ON CONFLICT (pubkey) DO UPDATE SET invoice = CASE
              WHEN registered_users.registered_at IS NULL THEN excluded.invoice
              ELSE registered_users.invoice
            END
            RETURNING invoice
        `, pubkey, inv.CheckingID); err != nil {
			json.NewEncoder(w).Encode(
				lnurl.ErrorResponse("failed to save invoice: " + err.Error()))
			return
		}

		if invoice != inv.CheckingID {
			json.NewEncoder(w).Encode(lnurl.ErrorResponse("user is already registered"))
			return
		}

		json.NewEncoder(w).Encode(lnurl.LNURLPayValues{
			LNURLResponse: lnurl.LNURLResponse{Status: "OK"},
			PR:            inv.Invoice,
			Routes:        make([]struct{}, 0),
			Disposable:    lnurl.TRUE,
			SuccessAction: lnurl.Action("Public Key "+pubkey+" registered!", ""),
		})
	}
}

func handlePaidInvoice(status relampago.InvoiceStatus) {
	if status.Exists && status.Paid {
		var pubkey string
		err := relay.db.Get(&pubkey, `
            UPDATE registered_users
            SET registered_at = extract(epoch from now())
            WHERE invoice = $1
            RETURNING pubkey
        `, status.CheckingID)
		if err != nil {
			relayer.Log.Warn().Err(err).Str("invoice", status.CheckingID).
				Msg("failed to register user")
		} else if pubkey != "" {
			relayer.Log.Warn().Str("pubkey", pubkey).Str("invoice", status.CheckingID).
				Msg("user registered")
		}
	}
}
