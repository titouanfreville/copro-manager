import {
  EmailAuthProvider,
  onAuthStateChanged,
  reauthenticateWithCredential,
  sendPasswordResetEmail,
  signInWithEmailAndPassword,
  signOut,
  updatePassword,
  type User,
} from "firebase/auth";
import { writable, type Readable } from "svelte/store";
import { browser } from "$app/environment";
import { firebaseAuth } from "./firebase";

type AuthState =
  | { status: "loading" }
  | { status: "signed-out" }
  | { status: "signed-in"; user: User }
  | { status: "config-error"; message: string };

const internal = writable<AuthState>({ status: "loading" });

if (browser) {
  // `firebaseAuth()` throws when PUBLIC_FIREBASE_* env vars are missing or
  // the Firebase SDK can't initialise. Without this guard the auth store
  // would stay at `loading` forever and every guarded route would render
  // a permanent "Chargement…" with zero diagnostic.
  try {
    onAuthStateChanged(firebaseAuth(), (user) => {
      internal.set(
        user ? { status: "signed-in", user } : { status: "signed-out" },
      );
    });
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    console.error("firebase init failed", err);
    internal.set({ status: "config-error", message });
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
  if (typeof err !== "object" || err === null || !("code" in err)) {
    return err instanceof Error ? err : new Error(String(err));
  }
  const code = (err as { code: string }).code;
  switch (code) {
    case "auth/invalid-email":
    case "auth/invalid-credential":
    case "auth/wrong-password":
    case "auth/user-not-found":
      return new Error("Email ou mot de passe incorrect.");
    case "auth/user-disabled":
      return new Error("Ce compte a été désactivé.");
    case "auth/too-many-requests":
      return new Error("Trop de tentatives. Réessayez dans quelques minutes.");
    case "auth/network-request-failed":
      return new Error(
        "Connexion réseau impossible. Vérifiez votre accès internet.",
      );
    case "auth/weak-password":
      return new Error("Mot de passe trop faible.");
    case "auth/requires-recent-login":
      return new Error("Connexion expirée. Reconnecte-toi puis réessaye.");
    default:
      return new Error("Opération impossible. Réessayez.");
  }
}

export async function logout(): Promise<void> {
  await signOut(firebaseAuth());
}

/**
 * Trigger Firebase's email-based password reset. Errors are swallowed
 * intentionally to avoid email enumeration — the caller shows the same
 * confirmation message regardless of whether the address is registered
 * (PRD FR5 / Story 1.4).
 */
export async function requestPasswordReset(email: string): Promise<void> {
  const trimmed = email.trim();
  if (!trimmed) throw new Error("Email requis.");
  try {
    await sendPasswordResetEmail(firebaseAuth(), trimmed);
  } catch (err) {
    // Surface only obviously bad-input errors; everything else gets
    // silenced so unregistered emails look identical to registered ones.
    if (typeof err === "object" && err !== null && "code" in err) {
      const code = (err as { code: string }).code;
      if (code === "auth/invalid-email") {
        throw new Error("Adresse email invalide.");
      }
      if (code === "auth/network-request-failed") {
        throw new Error(
          "Connexion réseau impossible. Vérifiez votre accès internet.",
        );
      }
      if (code === "auth/too-many-requests") {
        throw new Error("Trop de tentatives. Réessayez dans quelques minutes.");
      }
    }
    // Quiet fallback — let the caller show the generic confirmation.
  }
}

export async function idToken(): Promise<string | null> {
  const user = firebaseAuth().currentUser;
  if (!user) return null;
  return user.getIdToken();
}

/**
 * Change the signed-in user's password. Firebase requires a recent
 * sign-in for sensitive ops, so we reauthenticate first with the
 * current password — if the token aged out the call would otherwise
 * fail with `auth/requires-recent-login`.
 */
export async function changePassword(
  currentPassword: string,
  newPassword: string,
): Promise<void> {
  const user = firebaseAuth().currentUser;
  if (!user || !user.email) {
    throw new Error("Tu dois être connecté pour changer ton mot de passe.");
  }
  if (newPassword.length < 8) {
    throw new Error("Nouveau mot de passe trop court (8 caractères minimum).");
  }
  try {
    const cred = EmailAuthProvider.credential(user.email, currentPassword);
    await reauthenticateWithCredential(user, cred);
    await updatePassword(user, newPassword);
  } catch (err) {
    throw friendlyAuthError(err);
  }
}
