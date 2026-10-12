package main

import (
	"flag"
	"io"
	"net/http"
	"net/url"
)

func main() {
	port := flag.String("port", "", "listen port")
	name := flag.String("name", "go-service", "service name")
	upstream := flag.String("upstream", "", "optional upstream URL")
	flag.Parse()
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if *upstream != "" && r.URL.Path == "/" { proxy(w, r, *upstream); return }
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"service":"` + *name + `","status":"ok"}`))
	})
	if err := http.ListenAndServe("127.0.0.1:"+*port, h); err != nil { panic(err) }
}
func proxy(w http.ResponseWriter, r *http.Request, raw string) {
	u, err := url.Parse(raw); if err != nil { http.Error(w, err.Error(), 500); return }
	u.Path = r.URL.Path
	resp, err := http.Get(u.String()); if err != nil { http.Error(w, err.Error(), 502); return }
	defer resp.Body.Close(); w.WriteHeader(resp.StatusCode); _, _ = io.Copy(w, resp.Body)
}
