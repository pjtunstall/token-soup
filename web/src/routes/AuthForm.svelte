<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import type { SupabaseClient } from '@supabase/supabase-js';

	let { supabase }: { supabase: SupabaseClient } = $props();

	let email = $state('');
	let password = $state('');
	let authError = $state('');
	let authMode = $state<'sign_in' | 'sign_up'>('sign_in');
	let signUpMessage = $state('');
	let displayNameInput = $state('');

	async function handleAuthSubmit(e: SubmitEvent) {
		e.preventDefault();
		authError = '';
		signUpMessage = '';

		if (authMode === 'sign_up') {
			const name = displayNameInput.trim();
			if (!name) {
				authError = 'Please enter a display name.';
				return;
			}
			const { data, error } = await supabase.auth.signUp({
				email,
				password,
				options: {
					data: {
						display_name: name
					}
				}
			});
			if (error) {
				authError = error.message;
				return;
			}
			if (data.session) {
				return;
			}
			signUpMessage = 'Check your email for a confirmation link. After you confirm, sign in here.';
			password = '';
			return;
		}

		const { error } = await supabase.auth.signInWithPassword({ email, password });
		if (error) authError = error.message;
	}
</script>

<div class="flex max-w-sm flex-col gap-3">
	<div class="flex gap-2 text-sm">
		<button
			type="button"
			class="rounded-md px-2 py-1 underline-offset-4 hover:underline {authMode === 'sign_in'
				? 'font-medium text-foreground'
				: 'text-muted-foreground'}"
			onclick={() => {
				authMode = 'sign_in';
				authError = '';
				signUpMessage = '';
				displayNameInput = '';
			}}
		>
			Sign in
		</button>
		<span class="text-muted-foreground">·</span>
		<button
			type="button"
			class="rounded-md px-2 py-1 underline-offset-4 hover:underline {authMode === 'sign_up'
				? 'font-medium text-foreground'
				: 'text-muted-foreground'}"
			onclick={() => {
				authMode = 'sign_up';
				authError = '';
				signUpMessage = '';
			}}
		>
			Create account
		</button>
	</div>
	<form class="flex flex-col gap-2" onsubmit={handleAuthSubmit}>
		{#if authMode === 'sign_up'}
			<Input
				type="text"
				placeholder="Display name"
				bind:value={displayNameInput}
				autocomplete="nickname"
				maxlength={64}
				required
			/>
		{/if}
		<Input
			type="email"
			placeholder="Email"
			bind:value={email}
			autocomplete="email"
			required
		/>
		<Input
			type="password"
			placeholder="Password"
			bind:value={password}
			autocomplete={authMode === 'sign_up' ? 'new-password' : 'current-password'}
			minlength={authMode === 'sign_up' ? 6 : undefined}
			required
		/>
		{#if authError}
			<p class="text-sm text-destructive">{authError}</p>
		{/if}
		{#if signUpMessage}
			<p class="text-sm text-muted-foreground">{signUpMessage}</p>
		{/if}
		<Button type="submit">{authMode === 'sign_up' ? 'Create account' : 'Sign in'}</Button>
	</form>
</div>
