import { spawnSync } from "node:child_process";
import { existsSync, mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import * as path from "node:path";
import { fileURLToPath } from "node:url";

import type { ExtensionAPI, ExtensionContext } from "@earendil-works/pi-coding-agent";
import { Type } from "typebox";

const EXIT_CODE_ANNOTATIONS = 10;
const EXIT_CODE_ON_ANNOTATIONS_ENV = "REVDIFF_EXIT_CODE_ON_ANNOTATIONS";
const ANNOTATION_HEADER_RE = /^## (.+?)(?::(\d+)(?:-\d+)?)? \(([^)]+)\)$/;
const EXT_DIR = path.dirname(fileURLToPath(import.meta.url));
const PI_PLUGIN_ROOT = path.resolve(EXT_DIR, "..");
const REPO_ROOT = path.resolve(PI_PLUGIN_ROOT, "..", "..");
const DETECT_REF_SCRIPT = path.join(REPO_ROOT, ".claude-plugin", "skills", "revdiff", "scripts", "detect-ref.sh");

interface AnnotationItem {
	file: string;
	line?: number;
	kind: string;
	text: string;
}

interface ReviewResult {
	args: string[];
	argsText: string;
	label: string;
	rawOutput: string;
	annotations: AnnotationItem[];
}

interface LaunchSpec {
	args: string[];
	label: string;
}

interface SmartDetectResult {
	branch: string;
	mainBranch: string;
	isMain: boolean;
	hasUncommitted: boolean;
	useStaged: boolean;
	suggestedRef: string;
	needsAsk: boolean;
}

export default function revdiffExtension(pi: ExtensionAPI): void {
	async function startReview(ctx: ExtensionContext, launch: LaunchSpec): Promise<void> {
		if (!ctx.hasUI) {
			ctx.ui.notify("/revdiff requires the interactive TUI", "warning");
			return;
		}
		if (!ctx.isIdle()) {
			ctx.ui.notify("Wait for the current turn to finish before launching revdiff", "warning");
			return;
		}

		const result = await runDirectReview(ctx, launch);
		if (!result) {
			return;
		}

		if (result.annotations.length === 0) {
			ctx.ui.notify(`Review complete — no annotations for ${launch.label}`, "info");
			return;
		}

		const noun = result.annotations.length === 1 ? "annotation" : "annotations";
		ctx.ui.notify(`Captured ${result.annotations.length} ${noun}; sending to agent`, "info");
		pi.sendUserMessage(buildAgentPrompt(result));
	}

	pi.registerCommand("revdiff", {
		description: "Launch revdiff, capture annotations, and send them to the agent",
		handler: async (args, ctx) => {
			const launch = await resolveLaunchSpec(args, ctx);
			if (!launch) {
				return;
			}
			await startReview(ctx, launch);
		},
	});

	pi.registerTool({
		name: "revdiff_review",
		label: "revdiff review",
		description: "Launch revdiff in pi, capture interactive review annotations, and return them to the agent.",
		promptSnippet: "Launch an interactive revdiff review and return captured annotations.",
		promptGuidelines: [
			"Use revdiff_review only when the user explicitly asks for revdiff, an interactive annotation pass, or captured revdiff annotations inside pi.",
			"Do not use revdiff_review for ordinary autonomous code-review requests like 'review the code', 'review my changes', or 'review the diff'; inspect the code directly instead.",
			"After revdiff_review returns annotations, address them directly and rerun revdiff_review with the same args until no annotations are captured.",
		],
		parameters: Type.Object({
			args: Type.Optional(
				Type.String({
					description:
						"Optional revdiff arguments as a shell-like string, for example 'main', '--staged', '--only README.md', or '--all-files --exclude vendor'. Omit for smart detection.",
				}),
			),
		}),
		async execute(_toolCallId, params, _signal, onUpdate, ctx) {
			if (!ctx.hasUI) {
				return toolTextResult("revdiff_review requires the interactive pi TUI.");
			}

			const launch = await resolveLaunchSpec(params.args?.trim() ?? "", ctx);
			if (!launch) {
				return toolTextResult("Could not resolve a revdiff launch target.");
			}

			onUpdate?.({ content: [{ type: "text", text: `Launching revdiff for ${launch.label}...` }], details: null });
			const result = await runDirectReview(ctx, launch);
			if (!result) {
				return toolTextResult("revdiff review did not complete.");
			}

			if (result.annotations.length === 0) {
				return toolTextResult(`Review complete — no annotations for ${result.label}.`, result);
			}

			const noun = result.annotations.length === 1 ? "annotation" : "annotations";
			return toolTextResult(`Captured ${result.annotations.length} ${noun} for ${result.label}.`, result);
		},
	});
}

function toolTextResult(content: string, details?: ReviewResult) {
	return { content: [{ type: "text" as const, text: content }], details: details ?? null };
}

function buildAgentPrompt(result: ReviewResult): string {
	const rerun = result.argsText ? `Call revdiff_review with args: ${result.argsText}` : "Call revdiff_review without args";
	return [
		"revdiff captured annotations. Treat this as user feedback from an interactive review.",
		`Review target: ${result.label}`,
		`Original command: ${commandText(result.args)}`,
		`Rerun command: ${rerun}`,
		[
			"Workflow:",
			"- Classify annotations into explanation requests and code-change directives.",
			"- Answer explanation requests first.",
			"- If an explanation answer needs review, write it to a temporary markdown file and run revdiff_review with args: --only <tempfile> until clean.",
			"- Before editing repository files, list the planned file/code changes.",
			"- Apply code-change directives.",
			"- Rerun revdiff_review with the same args until no annotations are captured.",
			"- Add --untracked on reruns when agent-created files should be included.",
		].join("\n"),
		"Annotations:",
		result.rawOutput.trim(),
	]
		.filter(Boolean)
		.join("\n\n");
}

async function resolveLaunchSpec(rawArgs: string, ctx: ExtensionContext): Promise<LaunchSpec | undefined> {
	const trimmed = rawArgs.trim();
	if (!trimmed) {
		return detectSmartLaunch(ctx);
	}

	const split = shellSplit(trimmed);
	if (split.length === 0) {
		return detectSmartLaunch(ctx);
	}

	const allFiles = parseAllFilesShortcut(split.join(" "));
	if (allFiles) {
		return allFiles;
	}

	const tokens = sanitizeArgs(split);
	if (tokens.length === 0) {
		ctx.ui.notify("No revdiff arguments left after stripping --output", "warning");
		return undefined;
	}

	if (tokens.length === 1 && isFileReviewArg(tokens[0]!)) {
		const target = tokens[0]!;
		return { args: ["--only", target], label: target };
	}

	return { args: tokens, label: describeArgs(tokens) };
}

async function detectSmartLaunch(ctx: ExtensionContext): Promise<LaunchSpec | undefined> {
	const detected = detectSmartRef();
	if (!detected) {
		ctx.ui.notify("Not inside a git repo. Use /revdiff --only <file> to review a standalone file.", "warning");
		return undefined;
	}

	if (detected.needsAsk) {
		if (!detected.mainBranch) {
			ctx.ui.notify("Could not determine a revdiff target. Pass a ref or use /revdiff --only <file>.", "warning");
			return undefined;
		}
		const choice = await ctx.ui.select("revdiff", [
			"Review uncommitted changes only",
			`Review current branch against ${detected.mainBranch}`,
		]);
		if (!choice) {
			return undefined;
		}
		if (choice.startsWith("Review uncommitted")) {
			return uncommittedLaunchSpec(detected);
		}
		return { args: [detected.mainBranch], label: `${detected.branch} vs ${detected.mainBranch}` };
	}

	if (!detected.suggestedRef) {
		return uncommittedLaunchSpec(detected);
	}

	if (detected.mainBranch && detected.suggestedRef === detected.mainBranch && !detected.isMain) {
		return { args: [detected.mainBranch], label: `${detected.branch} vs ${detected.mainBranch}` };
	}

	return { args: [detected.suggestedRef], label: detected.suggestedRef };
}

function uncommittedLaunchSpec(detected: SmartDetectResult): LaunchSpec {
	if (detected.useStaged) {
		return { args: ["--staged"], label: "staged changes" };
	}
	return { args: [], label: "uncommitted changes" };
}

async function runDirectReview(ctx: ExtensionContext, launch: LaunchSpec): Promise<ReviewResult | undefined> {
	const revdiffBin = resolveRevdiffBin();
	if (!revdiffBin) {
		ctx.ui.notify("revdiff binary not found. Install it or set REVDIFF_BIN.", "error");
		return undefined;
	}

	const tempDir = mkdtempSync(path.join(tmpdir(), "revdiff-pi-"));
	const outputFile = path.join(tempDir, "annotations.txt");
	const commandArgs = [...launch.args, `--output=${outputFile}`];
	let launchError = "";
	let launchSignal = "";

	const exitCode = await ctx.ui.custom<number | null>((tui, _theme, _kb, done) => {
		tui.stop();
		process.stdout.write("\x1b[2J\x1b[H");
		const result = spawnSync(revdiffBin, commandArgs, {
			cwd: process.cwd(),
			env: withAnnotationExitCode(process.env),
			stdio: "inherit",
		});
		if (result.error) {
			launchError = result.error.message;
		}
		if (result.signal) {
			launchSignal = result.signal;
		}
		tui.start();
		tui.requestRender(true);
		done(result.status ?? 1);
		return { render: () => [], invalidate() {} };
	});

	const outputExists = existsSync(outputFile);
	const rawOutput = outputExists ? readFileSync(outputFile, "utf8").trim() : "";
	try {
		rmSync(tempDir, { recursive: true, force: true });
	} catch {
		// ignore temp cleanup failures
	}

	if (launchError) {
		ctx.ui.notify(`Failed to launch revdiff: ${launchError}`, "error");
		return undefined;
	}
	if (launchSignal) {
		ctx.ui.notify(`revdiff terminated by signal ${launchSignal}`, "warning");
		return undefined;
	}
	if (typeof exitCode !== "number") {
		ctx.ui.notify("revdiff review did not complete", "warning");
		return undefined;
	}
	if (!isRevdiffSuccess(exitCode)) {
		ctx.ui.notify(`revdiff exited with code ${exitCode}`, "warning");
		return undefined;
	}
	if (!outputExists && exitCode === EXIT_CODE_ANNOTATIONS) {
		ctx.ui.notify("revdiff reported annotations without writing output", "warning");
		return undefined;
	}

	return buildResult(launch, rawOutput);
}

function buildResult(launch: LaunchSpec, rawOutput: string): ReviewResult {
	return {
		args: [...launch.args],
		argsText: shellJoin(launch.args),
		label: launch.label,
		rawOutput,
		annotations: parseAnnotations(rawOutput),
	};
}

// request exit code 10 via env, not a CLI flag: an old revdiff binary silently
// ignores an unknown env var but hard-fails on an unknown flag
function withAnnotationExitCode(env: NodeJS.ProcessEnv): NodeJS.ProcessEnv {
	return { ...env, [EXIT_CODE_ON_ANNOTATIONS_ENV]: "true" };
}

function isRevdiffSuccess(exitCode: number): boolean {
	return exitCode === 0 || exitCode === EXIT_CODE_ANNOTATIONS;
}

function parseAnnotations(output: string): AnnotationItem[] {
	if (!output.trim()) {
		return [];
	}

	const annotations: AnnotationItem[] = [];
	let current:
		| {
				file: string;
				line?: number;
				kind: string;
				bodyLines: string[];
		  }
		| undefined;

	for (const rawLine of output.split(/\r?\n/)) {
		const match = ANNOTATION_HEADER_RE.exec(rawLine.trimEnd());
		if (!match) {
			if (current) {
				current.bodyLines.push(rawLine);
			}
			continue;
		}

		if (current) {
			annotations.push({
				file: current.file,
				line: current.line,
				kind: current.kind,
				text: current.bodyLines.join("\n").trim(),
			});
		}

		current = {
			file: match[1]!,
			line: match[2] ? Number.parseInt(match[2], 10) : undefined,
			kind: match[3]!,
			bodyLines: [],
		};
	}

	if (current) {
		annotations.push({
			file: current.file,
			line: current.line,
			kind: current.kind,
			text: current.bodyLines.join("\n").trim(),
		});
	}

	return annotations;
}

function describeArgs(args: string[]): string {
	if (args.length === 0) {
		return "uncommitted changes";
	}
	if (args.includes("--staged")) {
		return "staged changes";
	}
	if (args.includes("--all-files") || args.includes("-A")) {
		const excludes = collectFlagValues(args, "--exclude", "-X");
		if (excludes.length > 0) {
			return `all files, excluding ${excludes.join(", ")}`;
		}
		return "all files";
	}
	const onlyFiles = collectFlagValues(args, "--only", "-F");
	if (onlyFiles.length > 0) {
		return onlyFiles.length === 1 ? onlyFiles[0]! : `files: ${onlyFiles.join(", ")}`;
	}
	return shellJoin(args);
}

function parseAllFilesShortcut(raw: string): LaunchSpec | undefined {
	const match = /^all(?:-| )files(?:\s+exclude\s+(.+))?$/i.exec(raw);
	if (!match) {
		return undefined;
	}
	const args = ["--all-files"];
	const excludePart = match[1]?.trim();
	if (excludePart) {
		for (const part of excludePart.split(/\s*(?:,|and)\s*/)) {
			if (part) {
				args.push(`--exclude=${part}`);
			}
		}
	}
	return { args, label: describeArgs(args) };
}

function collectFlagValues(args: string[], longFlag: string, shortFlag: string): string[] {
	const values: string[] = [];
	for (let i = 0; i < args.length; i++) {
		const arg = args[i]!;
		if (arg === longFlag || arg === shortFlag) {
			if (i + 1 < args.length) {
				values.push(args[i + 1]!);
				i++;
			}
			continue;
		}
		if (arg.startsWith(`${longFlag}=`)) {
			values.push(arg.slice(longFlag.length + 1));
		}
	}
	return values;
}

function sanitizeArgs(args: string[]): string[] {
	const sanitized: string[] = [];
	let skipNext = false;
	for (const arg of args) {
		if (skipNext) {
			skipNext = false;
			continue;
		}
		if (arg === "-o" || arg === "--output") {
			skipNext = true;
			continue;
		}
		if (arg.startsWith("--output=")) {
			continue;
		}
		sanitized.push(arg);
	}
	return sanitized;
}

function isFileReviewArg(arg: string): boolean {
	if (arg.startsWith("-")) {
		return false;
	}
	if (existsSync(path.resolve(arg))) {
		return true;
	}
	if (arg.startsWith("/") || arg.startsWith("./")) {
		return true;
	}
	return arg.includes("/") && path.extname(arg) !== "" && !isGitRef(arg);
}

function isGitRef(arg: string): boolean {
	return gitOk(["rev-parse", "--verify", "--quiet", `${arg}^{commit}`]);
}

function shellSplit(input: string): string[] {
	const tokens: string[] = [];
	let current = "";
	let quote: '"' | "'" | undefined;
	let escaped = false;

	for (const char of input) {
		if (escaped) {
			current += char;
			escaped = false;
			continue;
		}
		if (char === "\\" && quote !== "'") {
			escaped = true;
			continue;
		}
		if (quote) {
			if (char === quote) {
				quote = undefined;
			} else {
				current += char;
			}
			continue;
		}
		if (char === '"' || char === "'") {
			quote = char;
			continue;
		}
		if (/\s/.test(char)) {
			if (current) {
				tokens.push(current);
				current = "";
			}
			continue;
		}
		current += char;
	}

	if (escaped) {
		current += "\\";
	}
	if (current) {
		tokens.push(current);
	}
	return tokens;
}

function shellJoin(args: string[]): string {
	return args.map(shellQuote).join(" ");
}

function commandText(args: string[]): string {
	const argsText = shellJoin(args);
	return argsText ? `revdiff ${argsText}` : "revdiff";
}

function shellQuote(arg: string): string {
	if (arg === "") {
		return "''";
	}
	if (/^[A-Za-z0-9_@%+=:,./-]+$/.test(arg)) {
		return arg;
	}
	return `'${arg.replace(/'/g, `'\\''`)}'`;
}

function resolveRevdiffBin(): string | undefined {
	const fromEnv = process.env.REVDIFF_BIN;
	if (fromEnv && existsSync(fromEnv)) {
		return fromEnv;
	}
	const fromPath = findInPath("revdiff");
	if (fromPath) {
		return fromPath;
	}
	const local = path.join(REPO_ROOT, ".bin", "revdiff");
	if (existsSync(local)) {
		return local;
	}
	return undefined;
}

function findInPath(binary: string): string | undefined {
	for (const dir of (process.env.PATH ?? "").split(path.delimiter)) {
		if (!dir) {
			continue;
		}
		const candidate = path.join(dir, binary);
		if (existsSync(candidate)) {
			return candidate;
		}
	}
	return undefined;
}

function detectSmartRef(): SmartDetectResult | undefined {
	return runDetectRefScript() ?? detectSmartRefFallback();
}

function runDetectRefScript(): SmartDetectResult | undefined {
	if (!existsSync(DETECT_REF_SCRIPT)) {
		return undefined;
	}
	const result = spawnSync(DETECT_REF_SCRIPT, [], { cwd: process.cwd(), encoding: "utf8" });
	if ((result.status ?? 1) !== 0) {
		return undefined;
	}

	const fields = new Map<string, string>();
	for (const line of (result.stdout ?? "").split("\n")) {
		const idx = line.indexOf(":");
		if (idx === -1) {
			continue;
		}
		fields.set(line.slice(0, idx).trim(), line.slice(idx + 1).trim());
	}
	if (fields.size === 0) {
		return undefined;
	}

	return {
		branch: fields.get("branch") ?? "HEAD",
		mainBranch: fields.get("main_branch") ?? "",
		isMain: fields.get("is_main") === "true",
		hasUncommitted: fields.get("has_uncommitted") === "true",
		useStaged: fields.get("use_staged") === "true",
		suggestedRef: fields.get("suggested_ref") ?? "",
		needsAsk: fields.get("needs_ask") === "true",
	};
}

function detectSmartRefFallback(): SmartDetectResult | undefined {
	if (gitStdout(["rev-parse", "--is-inside-work-tree"]) !== "true") {
		return undefined;
	}

	// detect no-commits state (fresh repo after git init)
	const hasCommits = gitOk(["rev-parse", "HEAD"]);
	const hasUncommitted = gitStdout(["status", "--porcelain"]).trim().length > 0;
	const hasUnstaged = !gitOk(["diff", "--quiet"]);
	const hasStaged = !gitOk(["diff", "--cached", "--quiet"]);
	const useStaged = hasUncommitted && hasStaged && !hasUnstaged;

	if (!hasCommits) {
		return {
			branch: "HEAD",
			mainBranch: "",
			isMain: false,
			hasUncommitted,
			useStaged: false,
			suggestedRef: "--all-files",
			needsAsk: false,
		};
	}

	const branch = gitStdout(["rev-parse", "--abbrev-ref", "HEAD"]) || "HEAD";
	const mainBranch = detectMainBranch();
	const isMain = Boolean(mainBranch) && branch === mainBranch;
	let suggestedRef = "";
	let needsAsk = false;

	if (isMain) {
		suggestedRef = hasUncommitted ? "" : "HEAD~1";
	} else if (hasUncommitted) {
		needsAsk = true;
	} else {
		suggestedRef = mainBranch;
	}

	return { branch, mainBranch, isMain, hasUncommitted, useStaged, suggestedRef, needsAsk };
}

function detectMainBranch(): string {
	const remoteHead = gitStdout(["symbolic-ref", "refs/remotes/origin/HEAD"]);
	if (remoteHead.startsWith("refs/remotes/origin/")) {
		return remoteHead.slice("refs/remotes/origin/".length);
	}
	if (gitOk(["show-ref", "--verify", "--quiet", "refs/heads/master"])) {
		return "master";
	}
	if (gitOk(["show-ref", "--verify", "--quiet", "refs/heads/main"])) {
		return "main";
	}
	return "";
}

function gitStdout(args: string[]): string {
	const result = spawnSync("git", args, { cwd: process.cwd(), encoding: "utf8" });
	if ((result.status ?? 1) !== 0) {
		return "";
	}
	return (result.stdout ?? "").trim();
}

function gitOk(args: string[]): boolean {
	return (spawnSync("git", args, { cwd: process.cwd(), stdio: "ignore" }).status ?? 1) === 0;
}
