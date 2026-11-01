import json, base64, random, string
from http.server import BaseHTTPRequestHandler, HTTPServer

BASE = "http://127.0.0.1:9100"
ISSUER = BASE

def b64(obj):
    return base64.urlsafe_b64encode(json.dumps(obj).encode()).decode().rstrip("=")

class H(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def _json(self, payload, code=200):
        body = json.dumps(payload).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        if self.path == "/.well-known/openid-configuration":
            self._json({
                "issuer": ISSUER,
                "authorization_endpoint": BASE + "/authorize",
                "token_endpoint": BASE + "/token",
                "userinfo_endpoint": BASE + "/userinfo",
                "jwks_uri": BASE + "/jwks",
                "end_session_endpoint": BASE + "/logout",
                "response_types_supported": ["code"],
                "subject_types_supported": ["public"],
                "id_token_signing_alg_values_supported": ["RS256"],
            })
        elif self.path == "/userinfo":
            self._json({
                "sub": "fake-subject-1",
                "email": "sso@example.com",
                "email_verified": True,
                "name": "SSO Tester",
                "preferred_username": "sso.tester",
            })
        else:
            self._json({"error": "not_found"}, 404)

    def do_POST(self):
        length = int(self.headers.get("Content-Length") or 0)
        form = self.rfile.read(length).decode()
        if self.path == "/token":
            if "code=" not in form:
                self._json({"error": "invalid_request"}, 400)
                return
            access = "".join(random.choices(string.ascii_letters + string.digits, k=32))
            self._json({
                "access_token": access,
                "token_type": "Bearer",
                "expires_in": 300,
                "id_token": "header." + b64({
                    "sub": "fake-subject-1",
                    "email": "sso@example.com",
                    "name": "SSO Tester",
                    "iss": ISSUER,
                }) + ".signature",
            })
        else:
            self._json({"error": "not_found"}, 404)

HTTPServer(("127.0.0.1", 9100), H).serve_forever()
# Dev helper: a minimal OpenID Provider used to exercise the SSO flow locally.
#
#   python3 scripts/fake-idp.py          # listens on http://127.0.0.1:9100
#   # then in the panel: Settings -> 单点登录, issuer http://127.0.0.1:9100,
#   # client id "2fauth", any secret, save, and click "用公司账号登录" on /login.
