package api

import "net/http"

// writeBoundedPreencodedJSONSuccess admits a complete, already-marshalled
// successful JSON document against an explicit caller-owned byte budget.
//
// It refuses an oversized/invalid document BEFORE writing a success status
// or any document bytes, and removes previously assigned public cache/ETag
// headers so intermediaries must not cache a size-rejection response.
//
// SECURITY BOUNDARY: the caller must provide trusted, marshalled JSON and a
// ratified budget. This function does NOT cap an upstream json.Marshal's
// allocation, compressed transport bytes, or a browser's decoded JSON heap.
// The package/replay routes must NOT call it until their server-owned byte
// budgets have been measured and ratified under Runtime Next #700.
func writeBoundedPreencodedJSONSuccess(
	w http.ResponseWriter, encoded []byte, maxBytes int,
) bool {
	if maxBytes < 0 || len(encoded) == 0 || len(encoded) > maxBytes {
		headers := w.Header()
		headers.Del("ETag")
		headers.Del("Last-Modified")
		headers.Del("Expires")
		headers.Del("Content-Length")
		headers.Del("Content-Encoding")
		headers.Set("Cache-Control", "private, no-store")
		writeErrorCode(
			w, http.StatusUnprocessableEntity,
			"successful_document_size_rejected",
			"published document could not be delivered",
		)
		return false
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(encoded)
	return true
}
