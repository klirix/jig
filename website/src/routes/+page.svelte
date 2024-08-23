<script>
	import hljs from 'highlight.js';
	import { CodeBlock, Tab, TabGroup, storeHighlightJs } from '@skeletonlabs/skeleton';

	storeHighlightJs.set(hljs);
	let tabSet = 0;
</script>

<svelte:head>
	<title>Jig - a dead simple deployment tool</title>
	<script async defer src="https://buttons.github.io/buttons.js"></script>
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
			<div class="px-2 md:px-0 text-xl mt-10 text-gray-800 dark:text-gray-200">
				<!-- Place this tag where you want the button to render. -->
				<div class="flex flex-row gap-4 items-center">
					<a href="https://github.com/klirix/jig">
						<svg xmlns="http://www.w3.org/2000/svg" width="40" height="40" viewBox="0 0 24 24"
							><path
								fill="currentColor"
								d="M12 2A10 10 0 0 0 2 12c0 4.42 2.87 8.17 6.84 9.5c.5.08.66-.23.66-.5v-1.69c-2.77.6-3.36-1.34-3.36-1.34c-.46-1.16-1.11-1.47-1.11-1.47c-.91-.62.07-.6.07-.6c1 .07 1.53 1.03 1.53 1.03c.87 1.52 2.34 1.07 2.91.83c.09-.65.35-1.09.63-1.34c-2.22-.25-4.55-1.11-4.55-4.92c0-1.11.38-2 1.03-2.71c-.1-.25-.45-1.29.1-2.64c0 0 .84-.27 2.75 1.02c.79-.22 1.65-.33 2.5-.33s1.71.11 2.5.33c1.91-1.29 2.75-1.02 2.75-1.02c.55 1.35.2 2.39.1 2.64c.65.71 1.03 1.6 1.03 2.71c0 3.82-2.34 4.66-4.57 4.91c.36.31.69.92.69 1.85V21c0 .27.16.59.67.5C19.14 20.16 22 16.42 22 12A10 10 0 0 0 12 2"
							/></svg
						>
					</a>
					<a
						class="github-button"
						href="https://github.com/klirix/jig"
						data-color-scheme="no-preference: light; light: light; dark: dark;"
						data-icon="octicon-star"
						data-size="large"
						data-show-count="true"
						aria-label="Star klirix/jig on GitHub">Star</a
					>
				</div>

				<h3 class=" my-3 text-2xl font-medium text-gray-900 dark:text-gray-100">What is Jig?</h3>
				<hr class=" mb-10" />
				<p>
					Jig is a dead simple deployment tool to automate routine work with Docker and Traefik to
					streamline running services on own virtual servers with following goals:
				</p>
				<ul class="space-y-6 ps-4 py-10 list-disc list-inside">
					<li>
						<span class="font-medium"> Bring Vercel DX to own servers: </span>
						Vercel is setting a standard for deployment tools for many years and aiming for anything
						less would mean disservice to everyone
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
				<p>
					DNS management is not builtin just yet so you need to point domain names you wish to use
					with Jig manually. 😔
				</p>
				<p>
					Thankfully it's mostly a one-time thing and you won't need to deal with it later. With
					Vercel domains it's as easy as an example below, but it depends on your provider
				</p>
				<CodeBlock
					language="bash"
					code="vc dns add <your base domain> <sub domain> A <your server public IP>"
				/>
				<p>
					As a last infra part you'll need to open up ports 80 (http) and 443 (https). Traefik
					handles https redirection and Let's Encrypt http challenges so you don't need to worry
					about unsecured connections
				</p>
				<p>Load and run startup\update script below</p>
				<div class="my-5">
					<CodeBlock
						language="bash"
						code="curl -fsSLO https://deploywithjig.askh.at/init.sh && bash init.sh"
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
				<div class="my-5">
					<CodeBlock
						language="bash"
						code="curl -fsSL https://deploywithjig.askh.at/install.sh | bash"
					/>
				</div>
				<p class="">
					After that just plug in the command you received in Server setup stage and start deploying
				</p>
			</div>
			<div class=" space-y-4 px-2 md:px-0 text-xl text-gray-800 dark:text-gray-200">
				<h3 class=" my-3 text-2xl font-medium text-gray-900 dark:text-gray-100">Start deploying</h3>
				<hr class=" mb-10" />
				<p>Initiate jig project and create the config</p>
				<CodeBlock language="bash" code="jig init" />
				<p>
					Deploy your project with a single command. Jig will pack the project, send it to the
					server and build it remotely
				</p>
				<CodeBlock language="bash" code="jig deploy" />
				<p>
					Or build it locally using docker and deploy the image to the server. This is useful for CI
					and to save resources on the server
				</p>
				<CodeBlock language="bash" code="jig deploy -l" />
				<p>Let Traefik fetch certificates if you deploy with TLS enabled and you're done</p>
			</div>
		</div>
	</section>
</main>
