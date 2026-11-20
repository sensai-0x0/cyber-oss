"""Non-functional phishing-panel visual fixture; credentials are never accepted."""

import argparse
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

PAGE = b"""<!doctype html><title>Lab sign-in fixture</title>
<h1>Phishing panel simulation</h1><p>Training fixture only. Form submission is disabled.</p>
<form><input disabled placeholder='simulated username'><input disabled type='password'>
<button disabled>Disabled</button></form>"""


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):  # noqa: N802 - stdlib HTTP handler API
        self.send_response(200)
        self.send_header("Content-Type", "text/html; charset=utf-8")
        self.send_header("Content-Length", str(len(PAGE)))
        self.end_headers()
        self.wfile.write(PAGE)

    def do_POST(self):  # noqa: N802 - reject rather than collect lab input
        self.send_error(403, "Fixture does not accept credentials")

    def log_message(self, *_args):
        return


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=8080)
    args = parser.parse_args()
    ThreadingHTTPServer((args.host, args.port), Handler).serve_forever()
