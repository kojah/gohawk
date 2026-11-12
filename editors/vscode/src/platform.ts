// The platforms an extension package is built for, and how each is named by
// VS Code, by Node, and by the Go toolchain. Pure data, shared by the
// extension and the packaging script.

export type Platform = {
	/** The VS Code Marketplace target, as `vsce package --target` takes it. */
	vsce: string;
	/** `process.platform` and `process.arch` on that target. */
	node: { platform: string; arch: string };
	/** GOOS and GOARCH for the gohawk binary bundled into the package. */
	go: { os: string; arch: string };
};

export const platforms: readonly Platform[] = [
	{
		vsce: 'linux-x64',
		node: { platform: 'linux', arch: 'x64' },
		go: { os: 'linux', arch: 'amd64' },
	},
	{
		vsce: 'linux-arm64',
		node: { platform: 'linux', arch: 'arm64' },
		go: { os: 'linux', arch: 'arm64' },
	},
	{
		vsce: 'darwin-x64',
		node: { platform: 'darwin', arch: 'x64' },
		go: { os: 'darwin', arch: 'amd64' },
	},
	{
		vsce: 'darwin-arm64',
		node: { platform: 'darwin', arch: 'arm64' },
		go: { os: 'darwin', arch: 'arm64' },
	},
	{
		vsce: 'win32-x64',
		node: { platform: 'win32', arch: 'x64' },
		go: { os: 'windows', arch: 'amd64' },
	},
	{
		vsce: 'win32-arm64',
		node: { platform: 'win32', arch: 'arm64' },
		go: { os: 'windows', arch: 'arm64' },
	},
];

/** The platform a Node process is running on, if packages are built for it. */
export function platformOf(platform: string, arch: string): Platform | undefined {
	return platforms.find(
		(candidate) => candidate.node.platform === platform && candidate.node.arch === arch,
	);
}

export function platformForTarget(vsceTarget: string): Platform | undefined {
	return platforms.find((candidate) => candidate.vsce === vsceTarget);
}

/** The binary's file name inside the package's bin directory. */
export function executableName(nodePlatform: string): string {
	return nodePlatform === 'win32' ? 'gohawk.exe' : 'gohawk';
}

export function errorMessage(error: unknown): string {
	return error instanceof Error ? error.message : String(error);
}
