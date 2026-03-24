const ENTRA_AUTHORITY = import.meta.env.VITE_ENTRA_AUTHORITY ?? 'https://login.microsoftonline.com';
const ENTRA_TENANT = import.meta.env.VITE_ENTRA_TENANT_ID ?? '';
const ENTRA_CLIENT_ID = import.meta.env.VITE_ENTRA_CLIENT_ID ?? '';

function getRedirectUri(): string {
  return `${window.location.origin}/auth/callback`;
}

export function getLoginUrl(): string {
  const params = new URLSearchParams({
    client_id: ENTRA_CLIENT_ID,
    response_type: 'code',
    redirect_uri: getRedirectUri(),
    scope: 'openid profile email',
    response_mode: 'query',
    state: crypto.randomUUID(),
  });
  return `${ENTRA_AUTHORITY}/${ENTRA_TENANT}/oauth2/v2.0/authorize?${params}`;
}

export function getCallbackRedirectUri(): string {
  return getRedirectUri();
}
