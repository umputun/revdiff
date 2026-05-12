import { spawnSync } from "node:child_process";
import { existsSync, mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import * as path from "node:path";
import { fileURLToPath } from "node:url";

import type {
	ExtensionAPI,
	ExtensionContext,
	Theme,
} from "@earendil-works/pi-coding-agent";
import { Box, matchesKey, Text, truncateToWidth, visibleWidth } from "@earendil-works/pi-tui";
import { Type } from "typebox";

const STATE_TYPE = "revdiff-state";
const LAST_LAUNCH_TYPE = "revdiff-last-launch";
const MESSAGE_TYPE = "revdiff-review";
const PANEL_PREVIEW_LINES = 18;
const WIDGET_PREVIEW_ITEMS = 3;
const ANNOTATION_HEADER_RE = /^## (.+?)(?::(\d+))? \(([^)]+)\)$/;
const EXT_DIR = path.dirname(fileURLToPath(import.meta.url));
const PI_PLUGIN_ROOT = path.resolve(EXT_DIR, "..");
const REPO_ROOT = path.resolve(PI_PLUGIN_ROOT, "..", "..");
const DETECT_REF_SCRIPT = path.join(REPO_ROOT, ".claude-plugin", "skills", "revdiff", "scripts", "detect-ref.sh");
const LAUNCH_REVDIFF_SCRIPT = path.join(
	REPO_ROOT,
	".claude-plugin",
	"skills",
	"revdiff",
	"scripts",
	"launch-revdiff.sh",
);

type LaunchMode = "direct" | "overlay";
type PanelAction = "apply" | "clear" | "rerun" | undefined;

interface AnnotationItem {
	file: string;
	line?: number;
	kind: string;
	text: string;
}

interface LaunchMemory {
	args: string[];
	label: string;
	mode: LaunchMode;
	createdAt: number;
}

interface ReviewState extends LaunchMemory {
	rawOutput: string;
	annotations: AnnotationItem[];
}

interface ClearedState {
	cleared: true;
}

interface LaunchSpec {
	args: string[];
	label: string;
	mode: LaunchMode;
}

interface SmartDetectResult {
	branch: string;
	mainBranch: string;
	isMain: boolean;
	hasUncommitted: boolean;
	suggestedRef: string;
	needsAsk: boolean;
}

interface GroupedAnnotations {
	file: string;
	items: Array<{ index: number; annotation: AnnotationItem }>;
}

interface ParsedPiArgs {
	args: string[];
	mode?: LaunchMode;
}

export default function revdiffExtension(pi: ExtensionAPI): void {
	let currentState: ReviewState | undefined;
	let lastLaunch: LaunchMemory | undefined;

	function setReviewState(ctx: ExtensionContext, state: ReviewState | undefined, persist = true): void {
		currentState = state;

		if (persist) {
			pi.appendEntry(STATE_TYPE, state ?? { cleared: true });
		}

		if (!ctx.hasUI) {
			return;
		}

		if (!state || state.annotations.length === 0) {
			ctx.ui.setStatus("revdiff", undefined);
			ctx.ui.setWidget("revdiff-results", undefined);
			return;
		}

		const theme = ctx.ui.theme;
		const count = state.annotations.length;
		const noun = count === 1 ? "annotation" : "annotations";
		const mode = state.mode === "overlay" ? "overlay" : "direct";
		ctx.ui.setStatus(
			"revdiff",
			`${theme.fg("warning", "rd")} ${theme.fg("dim", `${count} ${noun}`)} ${theme.fg("muted", `[${mode}]`)}`,
		);
		ctx.ui.setWidget("revdiff-results", buildWidgetLines(theme, state), { placement: "belowEditor" });
	}

	function setLastLaunch(launch: LaunchMemory | undefined, persist = true): void {
		lastLaunch = launch;
		if (persist) {
			pi.appendEntry(LAST_LAUNCH_TYPE, launch ?? { cleared: true });
		}
	}

	function restoreState(ctx: ExtensionContext): void {
		let restoredReview: ReviewState | undefined;
		let restoredLaunch: LaunchMemory | undefined;

		for (const entry of ctx.sessionManager.getBranch()) {
			if (entry.type !== "custom") {
				continue;
			}
			if (entry.customType === STATE_TYPE) {
				const data = entry.data as ReviewState | ClearedState | undefined;
				if (isReviewState(data)) {
					restoredReview = data;
				}
				if (isClearedState(data)) {
					restoredReview = undefined;
				}
			}
			if (entry.customType === LAST_LAUNCH_TYPE) {
				const data = entry.data as LaunchMemory | ClearedState | undefined;
				if (isLaunchMemory(data)) {
					restoredLaunch = data;
				}
				if (isClearedState(data)) {
					restoredLaunch = undefined;
				}
			}
		}

		lastLaunch = restoredLaunch;
		setReviewState(ctx, restoredReview, false);
	}

	async function startReview(ctx: ExtensionContext, launch: LaunchSpec): Promise<void> {
		if (!ctx.hasUI) {
			ctx.ui.notify("/revdiff requires the interactive TUI", "warning");
			return;
		}
		if (!ctx.isIdle()) {
			ctx.ui.notify("Wait for the current turn to finish before launching revdiff", "warning");
			return;
		}

		const result = await runReview(pi, ctx, launch);
		if (!result) {
			return;
		}

		setLastLaunch(
			{
				args: [...result.args],
				label: result.label,
				mode: result.mode,
				createdAt: result.createdAt,
			},
		);

		if (result.annotations.length === 0) {
			setReviewState(ctx, undefined);
			ctx.ui.notify(`Review complete — no annotations for ${launch.label}`, "info");
			return;
		}

		setReviewState(ctx, result);
		pi.sendMessage(
			{
				customType: MESSAGE_TYPE,
				content: `${result.annotations.length} annotation${result.annotations.length === 1 ? "" : "s"} captured for ${result.label}`,
				display: true,
				details: result,
			},
			{ triggerTurn: false },
		);

		ctx.ui.notify(`Captured ${result.annotations.length} annotation${result.annotations.length === 1 ? "" : "s"}`, "info");
		await handleAction(ctx, result, await openResultsPanel(ctx, result));
	}

	async function handleAction(ctx: ExtensionContext, state: ReviewState, action: PanelAction): Promise<void> {
		if (action === "clear") {
			setReviewState(ctx, undefined);
			ctx.ui.notify("Cleared revdiff annotations", "info");
			return;
		}
		if (action === "apply") {
			await applyReview(pi, ctx, state, () => setReviewState(ctx, undefined));
			return;
		}
		if (action === "rerun") {
			await startReview(ctx, { args: [...state.args], label: state.label, mode: state.mode });
		}
	}

	pi.registerMessageRenderer(MESSAGE_TYPE, (message, { expanded }, theme) => {
		const details = message.details as ReviewState | undefined;
		const lines = [theme.bold(theme.fg("accent", "revdiff")) + ` ${message.content}`];

		if (expanded && details) {
			for (const group of groupAnnotations(details.annotations)) {
				lines.push("");
				lines.push(theme.fg("muted", group.file));
				for (const { annotation } of group.items) {
					lines.push(`• ${formatWithinFile(annotation)}`);
					for (const line of wrapPlain(annotation.text, 72).slice(0, 2)) {
						lines.push(`  ${line}`);
					}
				}
			}
		}

		const box = new Box(1, 0, (text: string) => theme.bg("customMessageBg", text));
		box.addChild(new Text(lines.join("\n"), 0, 0));
		return box;
	});

	pi.registerCommand("revdiff", {
		description: "Launch revdiff, capture annotations, and show them in a side panel",
		handler: async (args, ctx) => {
			const launch = await resolveLaunchSpec(args, ctx);
			if (!launch) {
				return;
			}
			await startReview(ctx, launch);
		},
	});

	pi.registerCommand("revdiff-rerun", {
		description: "Rerun the last revdiff review with the remembered args",
		handler: async (args, ctx) => {
			if (!lastLaunch) {
				ctx.ui.notify("No previous revdiff run in this session branch", "info");
				return;
			}

			const parsed = parsePiArgs(args);
			const mode = normalizeLaunchMode(parsed.mode ?? lastLaunch.mode, ctx);
			if (!mode) {
				return;
			}

			await startReview(ctx, {
				args: [...lastLaunch.args],
				label: lastLaunch.label,
				mode,
			});
		},
	});

	pi.registerCommand("revdiff-results", {
		description: "Open the last revdiff results panel",
		handler: async (_args, ctx) => {
			if (!currentState) {
				ctx.ui.notify("No revdiff annotations captured in this session branch", "info");
				return;
			}
			await handleAction(ctx, currentState, await openResultsPanel(ctx, currentState));
		},
	});

	pi.registerCommand("revdiff-apply", {
		description: "Send the last revdiff annotations to the agent",
		handler: async (_args, ctx) => {
			if (!currentState) {
				ctx.ui.notify("No revdiff annotations captured in this session branch", "info");
				return;
			}
			await applyReview(pi, ctx, currentState, () => setReviewState(ctx, undefined));
		},
	});

	pi.registerCommand("revdiff-clear", {
		description: "Clear the stored revdiff annotations widget and panel state",
		handler: async (_args, ctx) => {
			setReviewState(ctx, undefined);
			ctx.ui.notify("Cleared revdiff annotations", "info");
		},
	});

	pi.registerTool({
		name: "revdiff_review",
		label: "revdiff review",
		description: "Launch revdiff in pi, capture interactive review annotations, and return them to the agent.",
		promptSnippet: "Launch an interactive revdiff review and return captured annotations.",
		promptGuidelines: [
			"Use revdiff_review when the user asks to review a diff, inspect changes interactively, or gather revdiff annotations inside pi.",
			"After revdiff_review returns annotations, address them directly instead of asking the user to run /revdiff or /revdiff-apply.",
		],
		parameters: Type.Object({
			args: Type.Optional(
				Type.String({
					description:
						"Optional revdiff arguments as a shell-like string, for example 'main', '--staged', '--only README.md', or '--all-files --exclude vendor'. Omit for smart detection.",
				}),
			),
			mode: Type.Optional(
				Type.Union([Type.Literal("direct"), Type.Literal("overlay")], {
					description: "Optional launch mode: 'direct' (default) or 'overlay'.",
				}),
			),
			openPanel: Type.Optional(
				Type.Boolean({ description: "Open the pi results panel after capture. Defaults to false for agent-driven reviews." }),
			),
		}),
		async execute(_toolCallId, params, _signal, onUpdate, ctx) {
			if (!ctx.hasUI) {
				return toolTextResult("revdiff_review requires the interactive pi TUI.");
			}

			// Tool execution runs during the agent turn, so ctx.isIdle() is expected
			// to be false here; the tool runner serializes this interactive call.
			const mode = params.mode;

			const rawArgs = [params.args?.trim() ?? "", mode ? `--pi-${mode}` : ""].filter(Boolean).join(" ");
			const launch = await resolveLaunchSpec(rawArgs, ctx);
			if (!launch) {
				return toolTextResult("Could not resolve a revdiff launch target.");
			}

			onUpdate?.({ content: [{ type: "text", text: `Launching revdiff for ${launch.label}...` }], details: null });
			const result = await runReview(pi, ctx, launch);
			if (!result) {
				return toolTextResult("revdiff review did not complete.");
			}

			setLastLaunch({
				args: [...result.args],
				label: result.label,
				mode: result.mode,
				createdAt: result.createdAt,
			});
			setReviewState(ctx, result.annotations.length === 0 ? undefined : result);

			if (params.openPanel === true && result.annotations.length > 0) {
				await openResultsPanel(ctx, result, { readOnly: true });
			}

			if (result.annotations.length === 0) {
				return toolTextResult(`Review complete — no annotations for ${result.label}.`, result);
			}

			const noun = result.annotations.length === 1 ? "annotation" : "annotations";
			return toolTextResult(`Captured ${result.annotations.length} ${noun} for ${result.label}.`, result);
		},
	});

	pi.on("session_start", async (_event, ctx) => {
		restoreState(ctx);
	});

	pi.on("session_tree", async (_event, ctx) => {
		restoreState(ctx);
	});
}

function toolTextResult(content: string, details?: ReviewState) {
	return { content: [{ type: "text" as const, text: content }], details: details ?? null };
}

async function applyReview(
	pi: ExtensionAPI,
	ctx: ExtensionContext,
	state: ReviewState,
	clearReviewState: () => void,
): Promise<void> {
	const prompt = buildApplyPrompt(state);
	if (ctx.isIdle()) {
		ctx.ui.notify("Sending revdiff annotations to the agent", "info");
		pi.sendUserMessage(prompt);
		clearReviewState();
		return;
	}

	ctx.ui.notify("Agent is busy — queued revdiff annotations as a follow-up", "info");
	pi.sendUserMessage(prompt, { deliverAs: "followUp" });
	clearReviewState();
}

async function resolveLaunchSpec(rawArgs: string, ctx: ExtensionContext): Promise<LaunchSpec | undefined> {
	const trimmed = rawArgs.trim();
	if (!trimmed) {
		const detected = await detectSmartLaunch(ctx);
		if (!detected) {
			return undefined;
		}
		const mode = normalizeLaunchMode(undefined, ctx);
		return mode ? { ...detected, mode } : undefined;
	}

	const parsed = parsePiArgs(trimmed);
	const mode = normalizeLaunchMode(parsed.mode, ctx);
	if (!mode) {
		return undefined;
	}

	if (parsed.args.length === 0) {
		const detected = await detectSmartLaunch(ctx);
		return detected ? { ...detected, mode } : undefined;
	}

	const allFiles = parseAllFilesShortcut(parsed.args.join(" "));
	if (allFiles) {
		return { ...allFiles, mode };
	}

	const tokens = sanitizeArgs(parsed.args);
	if (tokens.length === 0) {
		ctx.ui.notify("No revdiff arguments left after stripping --output", "warning");
		return undefined;
	}

	if (tokens.length === 1 && !tokens[0]!.startsWith("-") && existsSync(path.resolve(tokens[0]!))) {
		const target = tokens[0]!;
		return { args: ["--only", target], label: target, mode };
	}

	return { args: tokens, label: describeArgs(tokens), mode };
}

async function detectSmartLaunch(ctx: ExtensionContext): Promise<Omit<LaunchSpec, "mode"> | undefined> {
	const detected = detectSmartRef();
	if (!detected) {
		ctx.ui.notify("Not inside a git repo. Use /revdiff --only <file> to review a standalone file.", "warning");
		return undefined;
	}

	if (detected.needsAsk && detected.mainBranch) {
		const choice = await ctx.ui.select("revdiff", [
			"Review uncommitted changes only",
			`Review current branch against ${detected.mainBranch}`,
		]);
		if (!choice) {
			return undefined;
		}
		if (choice.startsWith("Review uncommitted")) {
			return { args: [], label: "uncommitted changes" };
		}
		return { args: [detected.mainBranch], label: `${detected.branch} vs ${detected.mainBranch}` };
	}

	if (!detected.suggestedRef) {
		return { args: [], label: "uncommitted changes" };
	}

	if (detected.mainBranch && detected.suggestedRef === detected.mainBranch && !detected.isMain) {
		return { args: [detected.mainBranch], label: `${detected.branch} vs ${detected.mainBranch}` };
	}

	return { args: [detected.suggestedRef], label: detected.suggestedRef };
}

async function runReview(pi: ExtensionAPI, ctx: ExtensionContext, launch: LaunchSpec): Promise<ReviewState | undefined> {
	pi.events.emit("revdiff:launch", { ...launch });
	if (launch.mode === "overlay") {
		return runOverlayReview(ctx, launch);
	}
	return runDirectReview(ctx, launch);
}

async function runDirectReview(ctx: ExtensionContext, launch: LaunchSpec): Promise<ReviewState | undefined> {
	const revdiffBin = resolveRevdiffBin();
	if (!revdiffBin) {
		ctx.ui.notify("revdiff binary not found. Install it or set REVDIFF_BIN.", "error");
		return undefined;
	}

	const tempDir = mkdtempSync(path.join(tmpdir(), "revdiff-pi-"));
	const outputFile = path.join(tempDir, "annotations.txt");
	const commandArgs = [...launch.args, `--output=${outputFile}`];
	let launchError = "";

	const exitCode = await ctx.ui.custom<number | null>((tui, _theme, _kb, done) => {
		tui.stop();
		process.stdout.write("\x1b[2J\x1b[H");
		const result = spawnSync(revdiffBin, commandArgs, {
			cwd: process.cwd(),
			env: process.env,
			stdio: "inherit",
		});
		if (result.error) {
			launchError = result.error.message;
		}
		tui.start();
		tui.requestRender(true);
		done(result.status ?? (result.error ? 1 : 0));
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
	if (typeof exitCode !== "number") {
		ctx.ui.notify("revdiff review did not complete", "warning");
		return undefined;
	}
	if (exitCode !== 0) {
		ctx.ui.notify(`revdiff exited with code ${exitCode}`, "warning");
		return undefined;
	}
	if (!outputExists) {
		ctx.ui.notify("revdiff completed without writing annotations output", "warning");
		return undefined;
	}

	return buildResult(launch, rawOutput);
}

async function runOverlayReview(ctx: ExtensionContext, launch: LaunchSpec): Promise<ReviewState | undefined> {
	const launcher = resolveLauncherScript();
	if (!launcher) {
		ctx.ui.notify("Overlay mode requested, but launch-revdiff.sh was not found", "error");
		return undefined;
	}

	const revdiffBin = resolveRevdiffBin();
	if (!revdiffBin) {
		ctx.ui.notify("revdiff binary not found. Install it or set REVDIFF_BIN.", "error");
		return undefined;
	}

	ctx.ui.notify("Launching revdiff overlay…", "info");
	const env = withRevdiffOnPath(process.env, revdiffBin);
	const result = spawnSync(launcher, launch.args, {
		cwd: process.cwd(),
		env,
		encoding: "utf8",
	});

	const stdout = (result.stdout ?? "").trim();
	const stderr = (result.stderr ?? "").trim();
	if (result.error) {
		ctx.ui.notify(`Failed to launch overlay: ${result.error.message}`, "error");
		return undefined;
	}
	if ((result.status ?? 0) !== 0) {
		ctx.ui.notify(stderr || `Overlay launcher exited with code ${result.status ?? 1}`, "error");
		return undefined;
	}

	return buildResult(launch, stdout);
}

function buildResult(launch: LaunchSpec, rawOutput: string): ReviewState {
	return {
		args: [...launch.args],
		label: launch.label,
		mode: launch.mode,
		rawOutput,
		annotations: parseAnnotations(rawOutput),
		createdAt: Date.now(),
	};
}

async function openResultsPanel(
	ctx: ExtensionContext,
	state: ReviewState,
	options: { readOnly?: boolean } = {},
): Promise<PanelAction> {
	return ctx.ui.custom<PanelAction>(
		(tui, theme, _kb, done) => new ReviewPanel(tui, theme, state, done, options.readOnly === true),
		{
			overlay: true,
			overlayOptions: {
				anchor: "right-center",
				width: "40%",
				minWidth: 48,
				margin: { right: 1 },
			},
		},
	);
}

class ReviewPanel {
	private selected = 0;
	private tui: { requestRender: (full?: boolean) => void };
	private theme: Theme;
	private state: ReviewState;
	private done: (value: PanelAction) => void;
	private readOnly: boolean;

	constructor(
		tui: { requestRender: (full?: boolean) => void },
		theme: Theme,
		state: ReviewState,
		done: (value: PanelAction) => void,
		readOnly: boolean,
	) {
		this.tui = tui;
		this.theme = theme;
		this.state = state;
		this.done = done;
		this.readOnly = readOnly;
	}

	handleInput(data: string): void {
		if (matchesKey(data, "escape") || data === "q" || data === "Q") {
			this.done(undefined);
			return;
		}
		if (matchesKey(data, "up") || data === "k") {
			this.selected = Math.max(0, this.selected - 1);
			this.tui.requestRender();
			return;
		}
		if (matchesKey(data, "down") || data === "j") {
			this.selected = Math.min(this.state.annotations.length - 1, this.selected + 1);
			this.tui.requestRender();
			return;
		}
		if (!this.readOnly && (matchesKey(data, "return") || data === "a" || data === "A")) {
			this.done("apply");
			return;
		}
		if (!this.readOnly && (data === "r" || data === "R")) {
			this.done("rerun");
			return;
		}
		if (!this.readOnly && (data === "c" || data === "C")) {
			this.done("clear");
		}
	}

	render(width: number): string[] {
		const th = this.theme;
		const innerW = Math.max(1, width - 2);
		const lines: string[] = [];
		const border = (s: string) => th.fg("border", s);
		const row = (s = "") => border("│") + padRight(truncateToWidth(s, innerW, "...", true), innerW) + border("│");

		lines.push(border(`╭${"─".repeat(innerW)}╮`));
		lines.push(
			row(
				`${th.fg("accent", "revdiff results")} ${th.fg("dim", `(${this.state.annotations.length})`)} ${th.fg("muted", `[${this.state.mode}]`)}`,
			),
		);
		lines.push(row(th.fg("dim", this.state.label)));
		lines.push(border(`├${"─".repeat(innerW)}┤`));

		for (const line of this.renderAnnotations(innerW)) {
			lines.push(row(line));
		}

		lines.push(border(`├${"─".repeat(innerW)}┤`));
		if (this.readOnly) {
			lines.push(row(th.fg("dim", "↑↓/j k move • Esc close")));
			lines.push(row(""));
		} else {
			lines.push(row(th.fg("dim", "↑↓/j k move • Enter/a apply • r rerun")));
			lines.push(row(th.fg("dim", "c clear • Esc close")));
		}
		lines.push(border(`╰${"─".repeat(innerW)}╯`));
		return lines;
	}

	private renderAnnotations(innerW: number): string[] {
		const rendered: string[] = [];
		let selectedAnchor = 0;
		const bodyWidth = Math.max(10, innerW - 4);

		for (const group of groupAnnotations(this.state.annotations)) {
			rendered.push(this.theme.bold(this.theme.fg("muted", group.file)));
			for (const { index, annotation } of group.items) {
				const selected = index === this.selected;
				if (selected) {
					selectedAnchor = rendered.length;
				}
				const prefix = selected ? this.theme.fg("accent", "▶") : this.theme.fg("dim", "•");
				const header = `${prefix} ${formatWithinFile(annotation)} ${this.theme.fg("dim", `[${annotation.kind}]`)}`;
				rendered.push(selected ? this.theme.fg("accent", header) : header);

				const bodyLines = wrapPlain(annotation.text, bodyWidth);
				const limit = selected ? 3 : 1;
				for (const line of bodyLines.slice(0, limit)) {
					rendered.push(selected ? `  ${line}` : this.theme.fg("dim", `  ${line}`));
				}
				if (bodyLines.length > limit) {
					rendered.push(this.theme.fg("dim", "  …"));
				}
				rendered.push("");
			}
		}

		const start = Math.max(0, Math.min(selectedAnchor - Math.floor(PANEL_PREVIEW_LINES / 3), Math.max(0, rendered.length - PANEL_PREVIEW_LINES)));
		const slice = rendered.slice(start, start + PANEL_PREVIEW_LINES);
		while (slice.length < PANEL_PREVIEW_LINES) {
			slice.push("");
		}
		return slice;
	}

	invalidate(): void {}
	dispose(): void {}
}

function buildWidgetLines(theme: Theme, state: ReviewState): string[] {
	const count = state.annotations.length;
	const noun = count === 1 ? "annotation" : "annotations";
	const groups = groupAnnotations(state.annotations);
	const lines = [
		`${theme.fg("accent", "revdiff")}: ${count} pending ${noun} ${theme.fg("dim", `(${state.label})`)} ${theme.fg("muted", `[${state.mode}]`)}`,
	];

	for (const group of groups.slice(0, WIDGET_PREVIEW_ITEMS)) {
		lines.push(`${theme.fg("muted", "•")} ${group.file} ${theme.fg("dim", `×${group.items.length}`)}`);
	}

	if (groups.length > WIDGET_PREVIEW_ITEMS) {
		lines.push(theme.fg("dim", `…and ${groups.length - WIDGET_PREVIEW_ITEMS} more files`));
	}

	lines.push(theme.fg("dim", "/revdiff-results • /revdiff-rerun • /revdiff-apply • /revdiff-clear"));
	return lines;
}

function buildApplyPrompt(state: ReviewState): string {
	return [
		"Please address the following revdiff annotations.",
		`Review target: ${state.label}`,
		`Launch mode: ${state.mode}`,
		`Original command: revdiff${state.args.length > 0 ? ` ${state.args.join(" ")}` : ""}`,
		"Start with a short plan, then make the necessary changes in the repository.",
		state.rawOutput.trim(),
	]
		.filter(Boolean)
		.join("\n\n");
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

function groupAnnotations(annotations: AnnotationItem[]): GroupedAnnotations[] {
	const groups: GroupedAnnotations[] = [];
	const byFile = new Map<string, GroupedAnnotations>();
	annotations.forEach((annotation, index) => {
		let group = byFile.get(annotation.file);
		if (!group) {
			group = { file: annotation.file, items: [] };
			byFile.set(annotation.file, group);
			groups.push(group);
		}
		group.items.push({ index, annotation });
	});
	return groups;
}

function formatWithinFile(annotation: AnnotationItem): string {
	if (annotation.kind === "file-level") {
		return "file note";
	}
	return annotation.line ? `line ${annotation.line}` : annotation.kind;
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
	return args.join(" ");
}

function parseAllFilesShortcut(raw: string): Omit<LaunchSpec, "mode"> | undefined {
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

function parsePiArgs(raw: string): ParsedPiArgs {
	const modeLess: string[] = [];
	let mode: LaunchMode | undefined;
	for (const token of shellSplit(raw)) {
		if (token === "--pi-overlay") {
			mode = "overlay";
			continue;
		}
		if (token === "--pi-direct") {
			mode = "direct";
			continue;
		}
		modeLess.push(token);
	}
	return { args: modeLess, mode };
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

function normalizeLaunchMode(mode: LaunchMode | undefined, ctx: ExtensionContext): LaunchMode | undefined {
	const envMode = process.env.REVDIFF_PI_MODE;
	const resolved = mode ?? (envMode === "overlay" ? "overlay" : "direct");
	if (resolved === "overlay" && !resolveLauncherScript()) {
		ctx.ui.notify("Overlay mode requested, but launch-revdiff.sh was not found", "error");
		return undefined;
	}
	return resolved;
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

function resolveLauncherScript(): string | undefined {
	if (existsSync(LAUNCH_REVDIFF_SCRIPT)) {
		return LAUNCH_REVDIFF_SCRIPT;
	}
	return findInPath("launch-revdiff.sh");
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

function withRevdiffOnPath(env: NodeJS.ProcessEnv, revdiffBin: string): NodeJS.ProcessEnv {
	const binDir = path.dirname(revdiffBin);
	const currentPath = env.PATH ?? "";
	return {
		...env,
		PATH: currentPath ? `${binDir}${path.delimiter}${currentPath}` : binDir,
	};
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

	if (!hasCommits) {
		return {
			branch: "HEAD",
			mainBranch: "",
			isMain: false,
			hasUncommitted,
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

	return { branch, mainBranch, isMain, hasUncommitted, suggestedRef, needsAsk };
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

function wrapPlain(text: string, width: number): string[] {
	const clean = text.replace(/\s+/g, " ").trim();
	if (!clean) {
		return [""];
	}
	const words = clean.split(" ");
	const lines: string[] = [];
	let current = "";

	for (const word of words) {
		const next = current ? `${current} ${word}` : word;
		if (visibleWidth(next) <= width) {
			current = next;
			continue;
		}
		if (current) {
			lines.push(current);
		}
		if (visibleWidth(word) <= width) {
			current = word;
			continue;
		}
		let remaining = word;
		while (visibleWidth(remaining) > width) {
			lines.push(remaining.slice(0, Math.max(1, width - 1)) + "…");
			remaining = remaining.slice(Math.max(1, width - 1));
		}
		current = remaining;
	}

	if (current) {
		lines.push(current);
	}
	return lines.length > 0 ? lines : [""];
}

function padRight(text: string, width: number): string {
	return text + " ".repeat(Math.max(0, width - visibleWidth(text)));
}

function isReviewState(data: unknown): data is ReviewState {
	return Boolean(
		data &&
			typeof data === "object" &&
			Array.isArray((data as ReviewState).annotations) &&
			typeof (data as ReviewState).rawOutput === "string" &&
			typeof (data as ReviewState).label === "string" &&
			((data as ReviewState).mode === "direct" || (data as ReviewState).mode === "overlay"),
	);
}

function isLaunchMemory(data: unknown): data is LaunchMemory {
	return Boolean(
		data &&
			typeof data === "object" &&
			Array.isArray((data as LaunchMemory).args) &&
			typeof (data as LaunchMemory).label === "string" &&
			((data as LaunchMemory).mode === "direct" || (data as LaunchMemory).mode === "overlay"),
	);
}

function isClearedState(data: unknown): data is ClearedState {
	return Boolean(data && typeof data === "object" && (data as ClearedState).cleared === true);
}
