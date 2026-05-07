<script lang="ts">
	import { goto } from '$app/navigation';
	import { authState, login } from '$lib/auth';

	let email = $state('');
	let password = $state('');
	let submitting = $state(false);
	let error = $state('');

	$effect(() => {
		if ($authState.status === 'signed-in') {
			goto('/');
		}
	});

	async function onSubmit(e: SubmitEvent) {
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
</script>

<main class="mx-auto flex min-h-screen max-w-md flex-col justify-center p-6">
	<h1 class="mb-1 text-2xl font-semibold">Copro Manager</h1>
	<p class="mb-8 text-sm text-slate-500">Connectez-vous pour continuer</p>

	<form class="space-y-4" onsubmit={onSubmit}>
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
	</form>
</main>
