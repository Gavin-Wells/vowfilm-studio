import { env } from 'cloudflare:workers';
async function proxy(request: Request) {
  const bindings = env as unknown as Record<string, string>;
  const base =
    bindings.GO_BACKEND_URL ||
    process.env.GO_BACKEND_URL ||
    'http://127.0.0.1:8097';
  const secret = bindings.GO_BACKEND_TOKEN || process.env.GO_BACKEND_TOKEN;
  const source = new URL(request.url);
  const origin = request.headers.get('origin');
  if (
    !['GET', 'HEAD'].includes(request.method) &&
    origin &&
    origin !== source.origin
  )
    return Response.json({ error: '请求来源不匹配' }, { status: 403 });
  if (!/^\/api\/(config|projects|media)(\/|$)/.test(source.pathname))
    return Response.json({ error: '接口不存在' }, { status: 404 });
  if (!secret)
    return Response.json(
      { error: '创作服务尚未配置连接密钥' },
      { status: 503 },
    );
  const headers = new Headers();
  for (const name of [
    'content-type',
    'range',
    'if-none-match',
    'if-modified-since',
  ]) {
    const value = request.headers.get(name);
    if (value) headers.set(name, value);
  }
  headers.set('X-Vowfilm-Token', secret);
  try {
    const response = await fetch(
      new URL(source.pathname + source.search, base),
      {
        method: request.method,
        headers,
        body: ['GET', 'HEAD'].includes(request.method)
          ? undefined
          : request.body,
        redirect: 'manual',
      },
    );
    if (response.status >= 300 && response.status < 400)
      return Response.json(
        { error: '创作服务返回了异常重定向' },
        { status: 502 },
      );
    const output = new Headers();
    for (const name of [
      'content-type',
      'content-length',
      'content-range',
      'accept-ranges',
      'content-disposition',
      'etag',
      'last-modified',
    ]) {
      const value = response.headers.get(name);
      if (value) output.set(name, value);
    }
    output.set('Cache-Control', 'private, no-store');
    output.set('X-Content-Type-Options', 'nosniff');
    return new Response(response.body, {
      status: response.status,
      headers: output,
    });
  } catch (error) {
    console.error(
      'Vowfilm proxy:',
      new URL(base).origin,
      error instanceof Error ? error.message : 'unknown',
    );
    return Response.json(
      { error: '创作服务暂时离线，请稍后重试。已保存的工程不会丢失。' },
      { status: 502 },
    );
  }
}
export const GET = proxy;
export const HEAD = proxy;
export const POST = proxy;
export const PATCH = proxy;
