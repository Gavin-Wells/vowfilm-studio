export async function api<T>(path: string, options?: RequestInit): Promise<T> {
  const headers = new Headers(options?.headers);
  if (!(options?.body instanceof FormData) && !headers.has('Content-Type'))
    headers.set('Content-Type', 'application/json');
  const response = await fetch(`/api/${path}`, {
    signal: AbortSignal.timeout(options?.method ? 120000 : 10000),
    ...options,
    headers,
  });
  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as {
      error?: string;
    };
    throw new Error(body.error || '创作服务暂时不可用，请稍后重试。');
  }
  return response.json();
}
