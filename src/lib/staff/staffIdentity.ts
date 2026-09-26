import { env } from '$env/dynamic/public';

const NAME_KEY = 'artist-colony-staff-name';
const SESSION_KEY = 'artist-colony-staff-session';
const TOKEN_KEY = 'artist-colony-staff-token';

export function apiBaseUrl(): string {
	return (env.PUBLIC_ORDER_ENDPOINT?.trim() || 'https://artist-colony.netrunner-meetup.com').replace(/\/$/, '');
}

export function getCurrentStaffName(): string {
	if (typeof localStorage !== 'undefined') {
		try {
			const stored = localStorage.getItem(NAME_KEY)?.trim();
			if (stored) return stored;
		} catch {
      // noop
		}
	}
	const fromEnv = env.PUBLIC_STAFF_NAME?.trim();
	if (fromEnv) return fromEnv;
	return 'Desk Staff';
}

export function setCurrentStaffName(name: string) {
	if (typeof localStorage === 'undefined') return;
	try {
		const trimmed = name.trim();
		if (trimmed) localStorage.setItem(NAME_KEY, trimmed);
		else localStorage.removeItem(NAME_KEY);
	} catch {
    // noop
	}
}

export function isStaffSession(): boolean {
	const fromEnv = env.PUBLIC_STAFF_MODE?.trim().toLowerCase();
	if (fromEnv === '1' || fromEnv === 'true' || fromEnv === 'yes') return true;
	if (typeof localStorage === 'undefined') return false;
	try {
		return localStorage.getItem(SESSION_KEY) === '1';
	} catch {
		return false;
	}
}

export function enableStaffSession() {
	if (typeof localStorage === 'undefined') return;
	try {
		localStorage.setItem(SESSION_KEY, '1');
	} catch {
    // noop
	}
	if (typeof document !== 'undefined') {
		document.documentElement.classList.add('staff-session');
	}
}

export function clearStaffSession() {
	if (typeof localStorage === 'undefined') return;
	try {
		localStorage.removeItem(SESSION_KEY);
	} catch {
    // noop
	}
	if (typeof document !== 'undefined') {
		document.documentElement.classList.remove('staff-session');
	}
}

export function syncStaffSessionClass() {
	if (typeof document === 'undefined') return;
	document.documentElement.classList.toggle('staff-session', isStaffSession());
}

export function getStaffToken(): string {
	if (typeof localStorage === 'undefined') return '';
	try {
		return localStorage.getItem(TOKEN_KEY) ?? '';
	} catch {
		return '';
	}
}

export function setStaffToken(token: string) {
	if (typeof localStorage === 'undefined') return;
	try {
		localStorage.setItem(TOKEN_KEY, token);
	} catch {
    // noop
	}
}

export function clearStaffToken() {
	if (typeof localStorage === 'undefined') return;
	try {
		localStorage.removeItem(TOKEN_KEY);
	} catch {
    // noop
	}
}

export type StaffLoginResult = {
	token: string;
	staffName: string;
	displayName: string;
};

export async function loginStaff(username: string, password: string): Promise<StaffLoginResult> {
	const res = await fetch(`${apiBaseUrl()}/staff/login`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ username, password })
	});
	const body = await res.json().catch(() => ({}));
	if (res.status === 401) {
		throw new Error(body.error || 'Invalid username or password.');
	}
	if (!res.ok) {
		throw new Error(body.error || 'Could not sign in.');
	}
	setStaffToken(body.token);
	setCurrentStaffName(body.displayName || body.staffName || username);
	enableStaffSession();
	return body;
}

export async function logoutStaff() {
	const token = getStaffToken();
	if (token) {
		void fetch(`${apiBaseUrl()}/staff/logout`, {
			method: 'POST',
			headers: { Authorization: `Bearer ${token}` }
		}).catch(() => {});
	}
	clearStaffToken();
	clearStaffSession();
}
