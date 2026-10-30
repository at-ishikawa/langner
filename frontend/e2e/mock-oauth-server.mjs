// Mock OAuth provider for e2e. config.e2e.yml points auth.oauth_endpoint here
// instead of Google, so the REAL sign-in round-trip (state cookie → provider →
// /auth/google/callback → code exchange → userinfo) runs in the browser without
// Google credentials. It approves every request as the allowlisted e2e user.
//
//   GET  /authorize  → 302 back to redirect_uri with ?code=…&state=… (no consent UI)
//   POST /token      → a fixed bearer access token
//   GET  /userinfo   → the e2e user's profile
import http from "node:http";

const PORT = Number(process.env.MOCK_OAUTH_PORT ?? 8090);
const ACCESS_TOKEN = "mock-oauth-access-token";
const EMAIL = process.env.E2E_EMAIL ?? "e2e@example.com";
// Same google_sub the seed gives the e2e user (syntheticSub in
// backend/internal/clicmd/auth.go), so signing in lands on the seeded account.
const USER = { sub: `e2e-test|${EMAIL}`, email: EMAIL, name: "E2E User" };

const server = http.createServer((req, res) => {
  const url = new URL(req.url ?? "/", `http://localhost:${PORT}`);
  switch (url.pathname) {
    case "/authorize": {
      const redirectURI = url.searchParams.get("redirect_uri");
      if (!redirectURI) {
        res.writeHead(400).end("missing redirect_uri");
        return;
      }
      const target = new URL(redirectURI);
      target.searchParams.set("code", "mock-oauth-code");
      target.searchParams.set("state", url.searchParams.get("state") ?? "");
      res.writeHead(302, { Location: target.toString() }).end();
      return;
    }
    case "/token":
      req.resume();
      req.on("end", () => {
        res.writeHead(200, { "Content-Type": "application/json" }).end(
          JSON.stringify({ access_token: ACCESS_TOKEN, token_type: "Bearer", expires_in: 3600 }),
        );
      });
      return;
    case "/userinfo":
      if (req.headers.authorization !== `Bearer ${ACCESS_TOKEN}`) {
        res.writeHead(401).end();
        return;
      }
      res.writeHead(200, { "Content-Type": "application/json" }).end(JSON.stringify(USER));
      return;
    case "/health":
      res.writeHead(200).end("ok");
      return;
    default:
      res.writeHead(404).end();
  }
});

server.listen(PORT, () => {
  console.error(`[mock-oauth] listening on http://localhost:${PORT}`);
});
