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
		<div class="pb-20">
			<h1
				class="unstyled text-9xl leading-snug pb-3 font-semibold text-gray-900 dark:text-gray-100"
			>
				Jig <span class="text-base text-gray-400 dark:text-gray-500">[ jig ]</span>
			</h1>
			<p class="text-gray-600 dark:text-gray-400">noun</p>
			<p class="ps-1 text-sm text-gray-400 dark:text-gray-500">
				a device that holds a piece of work and guides the tool operating on it.
			</p>
		</div>
		<div class="px-2 md:px-0 text-xl text-gray-800 dark:text-gray-200">
			<h3 class=" my-3 text-2xl font-medium text-gray-900 dark:text-gray-100">What is Jig?</h3>
			<hr class=" mb-10" />
			<p>
				Jig is a dead simple deployment tool to automate routine work with Docker and Traefik to
				streamline running services on own virtual servers with following goals:
			</p>
			<ul class="space-y-6 ps-4 py-10 list-disc list-inside">
				<li>
					<span class="font-medium"> Bring Vercel DX to own servers: </span>
					Vercel is setting a standard for deployment tools for many years and aiming for anything less
					would mean disservice to everyone
				</li>
				<li>
					<span class="font-medium"> Minimize error human error via automation: </span>
					It's very easy to forget one little command, skip a stop while removing a container and you
					get an error, start over and get sad. Automation solves this, but when bash scripts aren't
					enough stuff like Jig should be a great start
				</li>
				<li>
					<span class="font-medium"> Keep things fast: </span>
					From one line deployments to keeping disk writes to absolute minimum streaming data whereever
					this is possible, it is important to keep things fast for the comfortable
					<i>blazingly fast™</i> automation
				</li>
			</ul>
			<p>
				Jig was heavily inspired by <a class=" underline" href="https://vercel.com"
					>Zeit (older version of Vercel)</a
				>
				and
				<a class=" underline" href="https://github.com/exoframejs/exoframe">Exoframe.js</a> with focus
				on self-hosting and being minimal whenever this is possible.
			</p>
		</div>
		<div class="px-2 md:px-0 mt-10 text-xl text-gray-800 dark:text-gray-200">
			<h3 class=" my-3 text-2xl font-medium text-gray-900 dark:text-gray-100">Installation</h3>
			<hr class=" mb-10" />
			<p>Installation is in two steps:</p>
			<ul class="space-y-6 ps-4 py-10 list-decimal list-inside">
				<li>
					<span class="font-medium"> Server setup: </span>
					Run a startup script on the server to pull all relevant images and take off
				</li>
				<li>
					<span class="font-medium"> Client setup: </span>
					Download a client and get authentication ready
				</li>
			</ul>
		</div>
		<div class="px-2 md:px-0 mt-10 space-y-5 text-xl text-gray-800 dark:text-gray-200">
			<h3 class=" my-3 text-2xl font-medium text-gray-900 dark:text-gray-100">Server setup</h3>
			<p>
				Docker is a prerequisite, so&nbsp;<a
					class="underline"
					href="https://docs.docker.com/engine/install/"
					>ensure docker is available and running on your server</a
				>
			</p>
			<div class="my-5">
				<CodeBlock
					language="bash"
					code="wget -q https://deploywithjig.askh.at/install.sh && bash install.sh && rm install.sh"
				/>
			</div>
			<p class="">
				This will load traefik, jig, ask you for an email, jwt signing key, launch everything and
				spit out a command to run on your machine to login
			</p>
			<p>
				Login command will look something like <code class="rounded bg-gray-900 px-2"
					>jig login loooooong+code</code
				> keep it for later
			</p>
		</div>
		<div class="px-2 md:px-0 mt-10 space-y-5 text-xl text-gray-800 dark:text-gray-200">
			<h3 class=" my-3 text-2xl font-medium text-gray-900 dark:text-gray-100">Client setup</h3>
			<p>Any node package manager is a prerequisite</p>
			<div class="my-5" />
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
			<p class="">
				After that just plug in the command you received in Server setup stage and start deploying
			</p>
		</div>
	</section>
</main>
