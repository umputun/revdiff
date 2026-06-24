import { spawnSync } from "node:child_process";
import { existsSync, mkdtempSync, readFileSync, rmSync, statSync } from "node:fs";
import { homedir, tmpdir } from "node:os";
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
const DETECT_REF_SCRIPT = path.join(PI_PLUGIN_ROOT, "scripts", "detect-ref.sh");

interface AnnotationItem {
	file: string;
	line?: number;
	kind: string;
	text: string;
}

interface ReviewResult {
	args: string[];
	argsText: string;
	cwd: string;
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
	pi.registerCommand("revdiff", {
		description: "Resolve a revdiff request through the revdiff skill",
		handler: async (args) => {
			pi.sendUserMessage(skillCommand(args));
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
			"When revdiff_review captures annotations, read them from the tool result content; do not read revdiff history unless the tool reports an incomplete/missing-output result or the user explicitly asks for history.",
			"If revdiff_review returns no annotations, stop. Do not relaunch revdiff after any no-annotation result unless the user explicitly asks for another review.",
			"Rerun the original revdiff_review target only after code changes or when the user asks to continue reviewing; do not rerun it after explanation-only annotations are answered.",
		],
		parameters: Type.Object({
			args: Type.Optional(
				Type.String({
					description:
						"Optional revdiff arguments as a shell-like string, for example 'main', '--staged', '--only README.md', or '--all-files --exclude vendor'. Omit for smart detection.",
				}),
			),
			cwd: Type.Optional(
				Type.String({
					description: "Optional working directory for revdiff and git target resolution. Defaults to the current working directory.",
				}),
			),
		}),
		async execute(_toolCallId, params, _signal, onUpdate, ctx) {
			if (!ctx.hasUI) {
				return toolTextResult("revdiff_review requires the interactive pi TUI.");
			}

			const cwd = resolveReviewCwd(params.cwd, ctx.cwd);
			if (!cwd) {
				return toolTextResult("Could not resolve revdiff working directory.");
			}

			const launch = await resolveLaunchSpec(params.args?.trim() ?? "", ctx, cwd);
			if (!launch) {
				return toolTextResult("Could not resolve a revdiff launch target.");
			}

			onUpdate?.({ content: [{ type: "text", text: `Launching revdiff for ${launch.label} in ${cwd}...` }], details: null });
			const result = await runDirectReview(ctx, launch, cwd);
			if (!result) {
				return toolTextResult("revdiff review did not complete.");
			}

			if (result.annotations.length === 0) {
				return toolTextResult(`Review complete — no annotations for ${result.label}.`, result);
			}

			const noun = result.annotations.length === 1 ? "annotation" : "annotations";
			return toolTextResult(
				[`Captured ${result.annotations.length} ${noun} for ${result.label}.`, "", "Annotations:", result.rawOutput.trim()].join("\n"),
				result,
			);
		},
	});
}

function toolTextResult(content: string, details?: ReviewResult) {
	return { content: [{ type: "text" as const, text: content }], details: details ?? null };
}

function skillCommand(rawArgs: string): string {
	const trimmed = rawArgs.trim();
	return trimmed ? `/skill:revdiff ${trimmed}` : "/skill:revdiff";
}

function resolveReviewCwd(rawCwd: string | undefined, baseCwd: string): string | undefined {
	const expanded = expandHome(rawCwd?.trim() || baseCwd);
	const resolved = path.resolve(baseCwd, expanded);
	try {
		if (!statSync(resolved).isDirectory()) {
			return undefined;
		}
	} catch {
		return undefined;
	}
	return resolved;
}

function expandHome(value: string): string {
	if (value === "~") {
		return homedir();
	}
	if (value.startsWith("~/") || value.startsWith("~\\")) {
		return path.join(homedir(), value.slice(2));
	}
	return value;
}

async function resolveLaunchSpec(rawArgs: string, ctx: ExtensionContext, cwd: string): Promise<LaunchSpec | undefined> {
	const trimmed = rawArgs.trim();
	if (!trimmed) {
		return detectSmartLaunch(ctx, cwd);
	}

	const split = shellSplit(trimmed);
	if (split.length === 0) {
		return detectSmartLaunch(ctx, cwd);
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

	if (tokens.length === 1 && isFileReviewArg(tokens[0]!, cwd)) {
		const target = tokens[0]!;
		return { args: ["--only", target], label: target };
	}

	return { args: tokens, label: describeArgs(tokens) };
}

async function detectSmartLaunch(ctx: ExtensionContext, cwd: string): Promise<LaunchSpec | undefined> {
	const detected = detectSmartRef(cwd);
	if (!detected) {
		ctx.ui.notify("Not inside a supported VCS repo or smart detection is unavailable. Use /revdiff --only <file> to review a standalone file.", "warning");
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

async function runDirectReview(ctx: ExtensionContext, launch: LaunchSpec, cwd: string): Promise<ReviewResult | undefined> {
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
			cwd,
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

	return buildResult(launch, rawOutput, cwd);
}

function buildResult(launch: LaunchSpec, rawOutput: string, cwd: string): ReviewResult {
	return {
		args: [...launch.args],
		argsText: shellJoin(launch.args),
		cwd,
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

function isFileReviewArg(arg: string, cwd: string): boolean {
	if (arg.startsWith("-")) {
		return false;
	}
	if (existsSync(path.resolve(cwd, arg))) {
		return true;
	}
	return arg.startsWith("/") || arg.startsWith("./") || arg.startsWith("../");
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

function detectSmartRef(cwd: string): SmartDetectResult | undefined {
	return runDetectRefScript(cwd) ?? detectSmartRefFallback(cwd);
}

function runDetectRefScript(cwd: string): SmartDetectResult | undefined {
	if (!existsSync(DETECT_REF_SCRIPT)) {
		return undefined;
	}
	const result = spawnSync(DETECT_REF_SCRIPT, [], { cwd, encoding: "utf8" });
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

function detectSmartRefFallback(cwd: string): SmartDetectResult | undefined {
	if (gitStdout(["rev-parse", "--is-inside-work-tree"], cwd) !== "true") {
		return undefined;
	}

	// detect no-commits state (fresh repo after git init)
	const hasCommits = gitOk(["rev-parse", "HEAD"], cwd);
	const hasUncommitted = gitStdout(["status", "--porcelain"], cwd).trim().length > 0;
	const hasUnstaged = !gitOk(["diff", "--quiet"], cwd);
	const hasStaged = !gitOk(["diff", "--cached", "--quiet"], cwd);
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

	const branch = gitStdout(["rev-parse", "--abbrev-ref", "HEAD"], cwd) || "HEAD";
	const mainBranch = detectMainBranch(cwd);
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

function detectMainBranch(cwd: string): string {
	const remoteHead = gitStdout(["symbolic-ref", "refs/remotes/origin/HEAD"], cwd);
	if (remoteHead.startsWith("refs/remotes/origin/")) {
		return remoteHead.slice("refs/remotes/origin/".length);
	}
	if (gitOk(["show-ref", "--verify", "--quiet", "refs/heads/master"], cwd)) {
		return "master";
	}
	if (gitOk(["show-ref", "--verify", "--quiet", "refs/heads/main"], cwd)) {
		return "main";
	}
	return "";
}

function gitStdout(args: string[], cwd: string): string {
	const result = spawnSync("git", args, { cwd, encoding: "utf8" });
	if ((result.status ?? 1) !== 0) {
		return "";
	}
	return (result.stdout ?? "").trim();
}

function gitOk(args: string[], cwd: string): boolean {
	return (spawnSync("git", args, { cwd, stdio: "ignore" }).status ?? 1) === 0;
}
