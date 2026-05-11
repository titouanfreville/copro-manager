<script lang="ts">
	import { goto } from '$app/navigation';
	import { authState, changePassword } from '$lib/auth';

	let currentPwd = $state('');
	let newPwd = $state('');
	let confirmPwd = $state('');
	let submitting = $state(false);
	let error = $state('');
	let success = $state('');

	$effect(() => {
		if ($authState.status === 'signed-out') goto('/login');
	});

	async function onSubmit(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		success = '';
		if (newPwd !== confirmPwd) {
			error = 'La confirmation ne correspond pas au nouveau mot de passe.';
			return;
		}
		submitting = true;
		try {
			await changePassword(currentPwd, newPwd);
			success = 'Mot de passe mis à jour.';
			currentPwd = '';
			newPwd = '';
			confirmPwd = '';
		} catch (err) {
			error = err instanceof Error ? err.message : String(err);
		} finally {
			submitting = false;
		}
	}
</script>

<main class="mx-auto max-w-md p-6">
	<header class="mb-6">
		<h1 class="text-2xl font-semibold">Profil</h1>
		{#if $authState.status === 'signed-in'}
			<p class="text-sm text-slate-500">{$authState.user.email}</p>
		{/if}
	</header>

	<section class="rounded-lg border border-slate-200 bg-white p-4">
		<h2 class="mb-3 text-sm font-medium uppercase tracking-wide text-slate-500">
			Changer le mot de passe
		</h2>

		<form class="space-y-3" onsubmit={onSubmit}>
			<label class="block">
				<span class="text-sm font-medium">Mot de passe actuel</span>
				<input
					type="password"
					required
					autocomplete="current-password"
					bind:value={currentPwd}
					class="mt-1 w-full rounded-md border border-slate-300 px-3 py-2 focus:border-slate-900 focus:outline-none"
				/>
			</label>

			<label class="block">
				<span class="text-sm font-medium">Nouveau mot de passe</span>
				<input
					type="password"
					required
					autocomplete="new-password"
					minlength="8"
					bind:value={newPwd}
					class="mt-1 w-full rounded-md border border-slate-300 px-3 py-2 focus:border-slate-900 focus:outline-none"
				/>
				<span class="mt-1 block text-xs text-slate-500">8 caractères minimum.</span>
			</label>

			<label class="block">
				<span class="text-sm font-medium">Confirmer le nouveau mot de passe</span>
				<input
					type="password"
					required
					autocomplete="new-password"
					minlength="8"
					bind:value={confirmPwd}
					class="mt-1 w-full rounded-md border border-slate-300 px-3 py-2 focus:border-slate-900 focus:outline-none"
				/>
			</label>

			{#if error}
				<p role="alert" aria-live="assertive" class="text-sm text-red-600">{error}</p>
			{/if}
			{#if success}
				<p role="status" class="text-sm text-emerald-700">{success}</p>
			{/if}

			<button
				type="submit"
				disabled={submitting}
				class="w-full rounded-md bg-slate-900 py-2 font-medium text-white hover:bg-slate-800 disabled:opacity-50"
			>
				{submitting ? 'Mise à jour…' : 'Enregistrer'}
			</button>
		</form>
	</section>
</main>
