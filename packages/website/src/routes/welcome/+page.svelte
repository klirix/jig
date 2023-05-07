<script>
	import hljs from 'highlight.js';
	import { CodeBlock, Tab, TabGroup, storeHighlightJs } from '@skeletonlabs/skeleton';

	storeHighlightJs.set(hljs);
	let tabSet = 0;
</script>

<svelte:head>
	<title>Jig - a dead simple deployment tool</title>
</svelte:head>

<main class="w-full bg-gray-100 dark:bg-gray-900">
	<section
		class="bg-white dark:bg-gray-800 mx-auto container lg:max-w-4xl py-52 px-4 md:px-28 shadow-lg"
	>
		<div class="pb-10">
			<h1
				class="unstyled text-6xl sm:text-8xl md:text-9xl leading-snug pb-3 font-semibold text-gray-900 dark:text-gray-100"
			>
				Welcome!
			</h1>
		</div>
		<div class=" space-y-4 px-2 md:px-0 text-xl text-gray-800 dark:text-gray-200">
			<h3 class=" my-3 text-2xl font-medium text-gray-900 dark:text-gray-100">Start deploying</h3>
			<hr class=" mb-10" />
			<p>Install the client if you didn't do that already</p>
			<TabGroup>
				<Tab bind:group={tabSet} name="tab1" value={0}>npm</Tab>
				<Tab bind:group={tabSet} name="tab2" value={1}>pnpm</Tab>
				<Tab bind:group={tabSet} name="tab3" value={2}>yarn</Tab>
				<!-- Tab Panels --->
				<svelte:fragment slot="panel">
					{#if tabSet === 0}
						<CodeBlock language="bash" code="npm install -g jig-client" />
					{:else if tabSet === 1}
						<CodeBlock language="bash" code="pnpm add -g jig-client" />
					{:else if tabSet === 2}
						<CodeBlock language="bash" code="yarn global add jig-client" />
					{/if}
				</svelte:fragment>
			</TabGroup>
			<p>Initiate jig project and create the config</p>
			<CodeBlock language="bash" code="jig init" />
			<p>
				It will try to figure out what kind of project you're running and add a pre-made
				<code>Dockerfile</code> for your convenience, but you can create one yourself
			</p>
			<p>And deploy when you're ready</p>
			<CodeBlock language="bash" code="jig deploy" />
			<p>That's it!</p>
		</div>
	</section>
</main>
