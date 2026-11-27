// The join link the control room shows as a QR code: the join page with the
// lab code filled in.

/** Lab code as the join form takes it: capitals and digits, four characters. */
export const cleanLabCode = (raw: string) => raw.toUpperCase().replace(/[^A-Z0-9]/g, '').slice(0, 4)

export const joinLink = (origin: string, code: string) => `${origin}/join?code=${encodeURIComponent(code)}`

/** The lab code a join link carries, '' without one. */
export const codeFromSearch = (search: string) => cleanLabCode(new URLSearchParams(search).get('code') ?? '')
