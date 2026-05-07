import { initializeApp, type FirebaseApp } from 'firebase/app';
import {
	browserLocalPersistence,
	getAuth,
	setPersistence,
	type Auth
} from 'firebase/auth';
import { getFirestore, type Firestore } from 'firebase/firestore';
import {
	PUBLIC_FIREBASE_API_KEY,
	PUBLIC_FIREBASE_APP_ID,
	PUBLIC_FIREBASE_AUTH_DOMAIN,
	PUBLIC_FIREBASE_MESSAGING_SENDER_ID,
	PUBLIC_FIREBASE_PROJECT_ID,
	PUBLIC_FIREBASE_STORAGE_BUCKET
} from '$env/static/public';

let app: FirebaseApp | undefined;
let auth: Auth | undefined;
let firestore: Firestore | undefined;

interface FirebaseWebConfig {
	apiKey: string;
	authDomain: string;
	projectId: string;
	storageBucket: string;
	messagingSenderId: string;
	appId: string;
}

function config(): FirebaseWebConfig {
	const required = {
		PUBLIC_FIREBASE_API_KEY,
		PUBLIC_FIREBASE_APP_ID,
		PUBLIC_FIREBASE_MESSAGING_SENDER_ID,
		PUBLIC_FIREBASE_PROJECT_ID
	};

	for (const [name, value] of Object.entries(required)) {
		if (!value) {
			throw new Error(`Firebase config missing: ${name} is unset — see app/.env.sample`);
		}
	}

	return {
		apiKey: PUBLIC_FIREBASE_API_KEY,
		authDomain: PUBLIC_FIREBASE_AUTH_DOMAIN || `${PUBLIC_FIREBASE_PROJECT_ID}.firebaseapp.com`,
		projectId: PUBLIC_FIREBASE_PROJECT_ID,
		storageBucket: PUBLIC_FIREBASE_STORAGE_BUCKET || `${PUBLIC_FIREBASE_PROJECT_ID}.firebasestorage.app`,
		messagingSenderId: PUBLIC_FIREBASE_MESSAGING_SENDER_ID,
		appId: PUBLIC_FIREBASE_APP_ID
	};
}

export function firebaseApp(): FirebaseApp {
	if (!app) {
		app = initializeApp(config());
	}
	return app;
}

export function firebaseAuth(): Auth {
	if (!auth) {
		auth = getAuth(firebaseApp());
		// Persistence is fire-and-forget: if it rejects (Safari Private mode,
		// IndexedDB blocked, etc.) Firebase falls back to in-memory persistence
		// which still lets the user sign in for the current session. Log so a
		// "logged out on every refresh" report has a breadcrumb.
		setPersistence(auth, browserLocalPersistence).catch((err) => {
			console.warn('firebase persistence fallback to in-memory', err);
		});
	}
	return auth;
}

// Database name must match the Go API (api/conf/main.yml → firestore.database).
// Our project uses a literally-named "default" database (not the canonical
// "(default)" one), so we pass it explicitly here. Update both sides if you
// ever migrate to a different name.
const FIRESTORE_DATABASE_ID = 'default';

export function firebaseFirestore(): Firestore {
	if (!firestore) {
		firestore = getFirestore(firebaseApp(), FIRESTORE_DATABASE_ID);
	}
	return firestore;
}
