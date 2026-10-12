import argparse
from http.server import BaseHTTPRequestHandler, HTTPServer

parser = argparse.ArgumentParser()
parser.add_argument("--port", type=int, required=True)
parser.add_argument("--name", required=True)
args = parser.parse_args()

class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path in ("/healthz", "/"):
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(("{\"service\":\"%s\",\"status\":\"ok\"}" % args.name).encode())
        else:
            self.send_error(404)
    def log_message(self, *_): pass

HTTPServer(("127.0.0.1", args.port), Handler).serve_forever()
