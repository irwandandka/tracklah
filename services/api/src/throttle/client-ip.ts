type RequestLike = {
  ip?: string;
  headers: Record<string, string | string[] | undefined>;
};

// Behind Cloudflare Tunnel every request reaches the API from cloudflared
// on localhost, so req.ip is the same for everyone and a per-IP limit
// would throttle all riders together. Cloudflare puts the real client IP
// in CF-Connecting-IP.
//
// That header is only trustworthy when the API can't be reached except
// through the tunnel (the production compose binds it to 127.0.0.1), so
// it's opt-in via TRUST_CF_CONNECTING_IP. Otherwise anyone could send the
// header themselves and pick a fresh "IP" for every request.
export function clientIp(req: RequestLike, trustCloudflare: boolean): string {
  if (trustCloudflare) {
    const cf = req.headers['cf-connecting-ip'];
    const value = Array.isArray(cf) ? cf[0] : cf;
    if (value) return value;
  }
  return req.ip ?? 'unknown';
}
