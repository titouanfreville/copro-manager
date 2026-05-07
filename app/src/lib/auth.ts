import { onAuthStateChanged, signInWithEmailAndPassword, signOut, type User } from 'firebase/auth';
import { writable, type Readable } from 'svelte/store';
import { browser } from '$app/environment';
import { firebaseAuth } from './firebase';

type AuthState =
	| { status: 'loading' }
	| { status: 'signed-out' }
	| { status: 'signed-in'; user: User }
	| { status: 'config-error'; message: string };

const internal = writable<AuthState>({ status: 'loading' });

if (browser) {
	// `firebaseAuth()` throws when PUBLIC_FIREBASE_* env vars are missing or
	// the Firebase SDK can't initialise. Without this guard the auth store
	// would stay at `loading` forever and every guarded route would render
	// a permanent "Chargement…" with zero diagnostic.
	try {
		onAuthStateChanged(firebaseAuth(), (user) => {
			internal.set(user ? { status: 'signed-in', user } : { status: 'signed-out' });
		});
	} catch (err) {
		const message = err instanceof Error ? err.message : String(err);
		console.error('firebase init failed', err);
		internal.set({ status: 'config-error', message });
	}
}

export const authState: Readable<AuthState> = { subscribe: internal.subscribe };

export async function login(email: string, password: string): Promise<void> {
	const auth = firebaseAuth();
	try {
		await signInWithEmailAndPassword(auth, email, password);
	} catch (err) {
		throw friendlyAuthError(err);
	}
}

function friendlyAuthError(err: unknown): Error {
	if (typeof err !== 'object' || err === null || !('code' in err)) {
		return err instanceof Error ? err : new Error(String(err));
	}
	const code = (err as { code: string }).code;
	switch (code) {
		case 'auth/invalid-email':
		case 'auth/invalid-credential':
		case 'auth/wrong-password':
		case 'auth/user-not-found':
			return new Error('Email ou mot de passe incorrect.');
		case 'auth/user-disabled':
			return new Error('Ce compte a été désactivé.');
		case 'auth/too-many-requests':
			return new Error('Trop de tentatives. Réessayez dans quelques minutes.');
		case 'auth/network-request-failed':
			return new Error('Connexion réseau impossible. Vérifiez votre accès internet.');
		default:
			return new Error('Connexion impossible. Réessayez.');
	}
}

export async function logout(): Promise<void> {
	await signOut(firebaseAuth());
}

export async function idToken(): Promise<string | null> {
	const user = firebaseAuth().currentUser;
	if (!user) return null;
	return user.getIdToken();
}
