export async function responseErrorMessage(response: Response, fallback: string) {
  const body = (await response.json().catch(() => ({ error: fallback }))) as { error?: string }

  return body.error ?? fallback
}

export async function requestJSON<T>(path: string, init: RequestInit = {}, fallback: string): Promise<T> {
  const response = await fetch(path, { credentials: "include", ...init })

  if (!response.ok) {
    throw new Error(await responseErrorMessage(response, fallback))
  }

  return response.json() as Promise<T>
}

export async function requestEmpty(path: string, init: RequestInit = {}, fallback: string) {
  const response = await fetch(path, { credentials: "include", ...init })

  if (!response.ok && response.status !== 204) {
    throw new Error(await responseErrorMessage(response, fallback))
  }
}
