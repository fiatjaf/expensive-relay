package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strconv"

	"github.com/fiatjaf/go-lnurl"
	"github.com/gorilla/mux"
	"github.com/rdbell/relampago"
	"github.com/rdbell/relayer"
	"github.com/skip2/go-qrcode"
)

var priceSats int64

func handleWebpage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	// Params for template rendering
	var pageData struct {
		PriceSat int64
		Domain   string
		Amount   int64
		Pubkey   string
	}

	pageData.PriceSat = priceSats
	pageData.Amount = priceSats * 1000
	pageData.Domain = relay.Domain

	// Attempt to read pubkey from URL GET param
	pubkey := r.URL.Query().Get("pubkey")
	if v, err := hex.DecodeString(pubkey); err == nil && len(v) == 32 {
		pageData.Pubkey = pubkey
	}

	// Parse template
	tmpl, err := template.ParseFiles(relay.IndexTemplate)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(fmt.Sprintf(`Error parsing template: %s`, err.Error())))
		return
	}

	// Render and serve
	tmpl.Execute(w, pageData)
	return
}

// Handle register with HTML response
// For use with iframe embeds and privacy browsers with JS disaabled
func handleLnurlRegisterHTMLResponse(w http.ResponseWriter, r *http.Request) {
	response, err := register(r)
	w.Header().Set("Content-Type", "text/html")

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	// Validate pay values
	payValues, ok := response.(lnurl.LNURLPayValues)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("unable to parse payvalues response"))
		return
	}

	// Params for template rendering
	var pageData struct {
		InvoiceRaw string
		InvoiceQR  string
	}

	// Generate QR code
	png, _ := qrcode.Encode(payValues.PR, qrcode.Medium, 256)
	pageData.InvoiceRaw = payValues.PR
	pageData.InvoiceQR = base64.StdEncoding.EncodeToString(png)

	// Parse template
	tmpl, err := template.ParseFiles(relay.InvoiceTemplate)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fmt.Sprintf(`Error parsing template: %s`, err.Error())))
		return
	}

	// Render and serve
	w.Header().Set("Content-Type", "text/html")
	tmpl.Execute(w, pageData)
	return
}

// Handle register with JSON response
func handleLnurlRegisterJSONResponse(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	response, err := register(r)
	if err != nil {
		json.NewEncoder(w).Encode(lnurl.ErrorResponse(err.Error()))
		return
	}

	json.NewEncoder(w).Encode(response)
	return

}

func register(r *http.Request) (response interface{}, err error) {
	// Attempt to read pubkey from mux vars
	pubkey := mux.Vars(r)["pubkey"]
	if v, err := hex.DecodeString(pubkey); err != nil || len(v) != 32 {
		// Attempt to read pubkey from URL GET param
		pubkey = r.URL.Query().Get("pubkey")
		if v, err := hex.DecodeString(pubkey); err != nil || len(v) != 32 {
			return nil, errors.New("invalid pubkey " + pubkey)
		}
	}

	metadata, _ := json.Marshal([][]string{
		{"text/plain", "registration for pubkey " + pubkey},
	})

	amount := r.URL.Query().Get("amount")
	if amount == "" {
		return lnurl.LNURLPayParams{
			LNURLResponse: lnurl.LNURLResponse{Status: "OK"},
			Callback: fmt.Sprintf("https://%s/.well-known/lnurlp/%s",
				relay.Domain, pubkey),
			MinSendable:     priceSats * 1000,
			MaxSendable:     priceSats * 1000,
			EncodedMetadata: string(metadata),
			Tag:             "payRequest",
		}, nil
	}

	msat, err := strconv.ParseInt(amount, 10, 64)
	if err != nil || msat != priceSats*1000 {
		return nil, errors.New("invalid amount " + amount)
	}

	h := sha256.Sum256(metadata)
	inv, err := relay.ln.CreateInvoice(relampago.InvoiceParams{
		Msatoshi:        int64(msat),
		DescriptionHash: h[:],
	})
	if err != nil {
		return nil, errors.New("failed to create invoice: " + err.Error())
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
		return nil, errors.New("failed to save invoice: " + err.Error())
	}

	if invoice != inv.CheckingID {
		return nil, errors.New("user is already registered")
	}

	return lnurl.LNURLPayValues{
		LNURLResponse: lnurl.LNURLResponse{Status: "OK"},
		PR:            inv.Invoice,
		Routes:        make([]struct{}, 0),
		Disposable:    lnurl.TRUE,
		SuccessAction: lnurl.Action("Public Key "+pubkey+" registered!", ""),
	}, nil
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

func handleCheckRegistration(w http.ResponseWriter, r *http.Request) {
	// TODO: set http status codes?
	w.Header().Set("Content-Type", "application/json")
	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	response.Success = false
	response.Message = ""

	// Attempt to read pubkey from mux vars
	pubkey := mux.Vars(r)["pubkey"]
	if v, err := hex.DecodeString(pubkey); err != nil || len(v) != 32 {
		response.Message = "invalid pubkey"
		json.NewEncoder(w).Encode(response)
		return
	}

	// Query DB for registration status
	var registeredAt int
	if err := relay.db.Get(&registeredAt, `
           SELECT registered_at FROM registered_users WHERE pubkey = $1
        `, pubkey); err != nil {
		response.Message = "pubkey is not registered"
		json.NewEncoder(w).Encode(response)
		return
	}

	if registeredAt < 1 {
		response.Message = "pubkey is not registered"
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Message = fmt.Sprintf("pubkey registered at timestamp %d", registeredAt)
	response.Success = true
	json.NewEncoder(w).Encode(response)
	return
}
