<script lang="ts">
	import { goto } from '$app/navigation';
	import { authState } from '$lib/auth';

	$effect(() => {
		if ($authState.status === 'signed-out') {
			goto('/login');
		} else if ($authState.status === 'signed-in') {
			goto('/expenses');
		}
	});
</script>

<main class="mx-auto max-w-2xl p-6">
	<p class="text-slate-500">Chargement…</p>
</main>
