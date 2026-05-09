<script lang="ts">
	import { goto } from '$app/navigation';
	import { authState, login, requestPasswordReset } from '$lib/auth';

	type Mode = 'sign-in' | 'reset';
	let mode = $state<Mode>('sign-in');

	let email = $state('');
	let password = $state('');
	let submitting = $state(false);
	let error = $state('');
	// Same confirmation message for registered + unregistered emails — no
	// enumeration (Story 1.4 acceptance criterion).
	let resetSent = $state(false);

	$effect(() => {
		if ($authState.status === 'signed-in') {
			goto('/');
		}
	});

	function switchMode(next: Mode) {
		mode = next;
		error = '';
		resetSent = false;
	}

	async function onSignIn(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		submitting = true;
		try {
			await login(email, password);
		} catch (err) {
			error = err instanceof Error ? err.message : String(err);
		} finally {
			submitting = false;
		}
	}

	async function onReset(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		submitting = true;
		try {
			await requestPasswordReset(email);
			resetSent = true;
		} catch (err) {
			error = err instanceof Error ? err.message : String(err);
		} finally {
			submitting = false;
		}
	}
</script>

<main class="mx-auto flex min-h-screen max-w-md flex-col justify-center p-6">
	<h1 class="mb-1 text-2xl font-semibold">Copro Manager</h1>
	<p class="mb-8 text-sm text-slate-500">
		{mode === 'sign-in' ? 'Connectez-vous pour continuer' : 'Réinitialiser votre mot de passe'}
	</p>

	{#if mode === 'sign-in'}
		<form class="space-y-4" onsubmit={onSignIn}>
			<label class="block">
				<span class="text-sm font-medium">Email</span>
				<input
					type="email"
					required
					autocomplete="email"
					bind:value={email}
					class="mt-1 w-full rounded-md border border-slate-300 px-3 py-2 focus:border-slate-900 focus:outline-none"
				/>
			</label>

			<label class="block">
				<span class="text-sm font-medium">Mot de passe</span>
				<input
					type="password"
					required
					autocomplete="current-password"
					bind:value={password}
					class="mt-1 w-full rounded-md border border-slate-300 px-3 py-2 focus:border-slate-900 focus:outline-none"
				/>
			</label>

			{#if error}
				<p role="alert" aria-live="assertive" class="text-sm text-red-600">{error}</p>
			{/if}

			<button
				type="submit"
				disabled={submitting}
				class="w-full rounded-md bg-slate-900 py-2 font-medium text-white hover:bg-slate-800 disabled:opacity-50"
			>
				{submitting ? 'Connexion…' : 'Se connecter'}
			</button>

			<div class="pt-1 text-center">
				<button
					type="button"
					class="text-sm text-slate-600 underline underline-offset-2 hover:text-slate-900"
					onclick={() => switchMode('reset')}
				>
					Mot de passe oublié ?
				</button>
			</div>
		</form>
	{:else}
		<form class="space-y-4" onsubmit={onReset}>
			<label class="block">
				<span class="text-sm font-medium">Email</span>
				<input
					type="email"
					required
					autocomplete="email"
					bind:value={email}
					class="mt-1 w-full rounded-md border border-slate-300 px-3 py-2 focus:border-slate-900 focus:outline-none"
				/>
			</label>

			{#if error}
				<p role="alert" aria-live="assertive" class="text-sm text-red-600">{error}</p>
			{/if}

			{#if resetSent}
				<p
					role="status"
					aria-live="polite"
					class="rounded-md border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-800"
				>
					Si cet email existe, un lien de réinitialisation vient d'être envoyé.
				</p>
			{:else}
				<button
					type="submit"
					disabled={submitting}
					class="w-full rounded-md bg-slate-900 py-2 font-medium text-white hover:bg-slate-800 disabled:opacity-50"
				>
					{submitting ? 'Envoi…' : 'Envoyer le lien'}
				</button>
			{/if}

			<div class="pt-1 text-center">
				<button
					type="button"
					class="text-sm text-slate-600 underline underline-offset-2 hover:text-slate-900"
					onclick={() => switchMode('sign-in')}
				>
					← Retour à la connexion
				</button>
			</div>
		</form>
	{/if}
</main>
