import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { tryRefreshSession } from "./refresh";
import { getAccessToken, clearAccessToken } from "./authToken";

// refresh.ts exchanges the HttpOnly refresh cookie for a new access token. JS
// never reads the refresh token: the request carries no body and relies on
// `credentials:"include"` to attach the cookie.
describe("tryRefreshSession", () => {
  beforeEach(() => {
    clearAccessToken();
  });
  afterEach(() => {
    vi.restoreAllMocks();
    clearAccessToken();
  });

  it("POSTs the cookie (credentials:include, no body) and stores the new access token", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ access_token: "new-access" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const ok = await tryRefreshSession();

    expect(ok).toBe(true);
    expect(getAccessToken()).toBe("new-access");
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("/auth/token/refresh");
    expect(init.method).toBe("POST");
    expect(init.credentials).toBe("include");
    expect(init.body).toBeUndefined(); // refresh token never sent by JS
  });

  it("returns false and clears the access token when there is no valid cookie (401)", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("", { status: 401 })));
    const ok = await tryRefreshSession();
    expect(ok).toBe(false);
    expect(getAccessToken()).toBeNull();
  });

  it("serializes concurrent refreshes into a single request (rotation-safe)", async () => {
    let resolve!: (r: Response) => void;
    const fetchMock = vi.fn().mockReturnValue(new Promise<Response>((r) => (resolve = r)));
    vi.stubGlobal("fetch", fetchMock);

    const a = tryRefreshSession();
    const b = tryRefreshSession();
    resolve(
      new Response(JSON.stringify({ access_token: "shared" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    const [ra, rb] = await Promise.all([a, b]);

    expect(ra).toBe(true);
    expect(rb).toBe(true);
    expect(fetchMock).toHaveBeenCalledTimes(1); // one in-flight refresh shared
  });
});
