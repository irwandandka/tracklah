import { clientIp } from './client-ip';

describe('clientIp', () => {
  const viaTunnel = {
    ip: '127.0.0.1',
    headers: { 'cf-connecting-ip': '203.0.113.7' },
  };

  it('uses CF-Connecting-IP when Cloudflare is trusted', () => {
    expect(clientIp(viaTunnel, true)).toBe('203.0.113.7');
  });

  it('ignores CF-Connecting-IP when Cloudflare is not trusted', () => {
    expect(clientIp(viaTunnel, false)).toBe('127.0.0.1');
  });

  it('falls back to req.ip when the header is missing', () => {
    expect(clientIp({ ip: '10.0.0.5', headers: {} }, true)).toBe('10.0.0.5');
  });
});
