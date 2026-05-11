import type { ExtensionAPI, ExtensionContext } from "@earendil-works/pi-coding-agent";

const CONFIG_TYPE = "revdiff-reminder-config";
const STATUS_KEY = "revdiff-reminder";
const LAST_LAUNCH_TYPE = "revdiff-last-launch";

interface ReminderConfig {
	enabled: boolean;
}

interface LaunchMemory {
	args: string[];
	label: string;
	mode: "direct" | "overlay";
	createdAt: number;
}

export default function revdiffPostEditReminder(pi: ExtensionAPI): void {
	let enabled = false;
	let editCallsInRun = 0;
	let lastUIContext: ExtensionContext | undefined;

	function rememberCtx(ctx: ExtensionContext): void {
		if (ctx.hasUI) {
			lastUIContext = ctx;
		}
	}

	function persistConfig(): void {
		pi.appendEntry(CONFIG_TYPE, { enabled });
	}

	function clearReminder(): void {
		if (!lastUIContext?.hasUI) {
			return;
		}
		lastUIContext.ui.setStatus(STATUS_KEY, undefined);
	}

	function resolveSuggestedCommand(ctx: ExtensionContext): string {
		const launch = getLastLaunch(ctx);
		if (!launch) {
			return "/revdiff";
		}
		if (launch.mode === "overlay") {
			return "/revdiff-rerun --pi-overlay";
		}
		return "/revdiff-rerun";
	}

	function updateStatus(ctx: ExtensionContext): void {
		rememberCtx(ctx);
		if (!enabled || editCallsInRun === 0) {
			clearReminder();
			return;
		}
		if (!ctx.hasUI) {
			return;
		}
		const command = resolveSuggestedCommand(ctx);
		ctx.ui.setStatus(
			STATUS_KEY,
			`${ctx.ui.theme.fg("warning", "review?")} ${ctx.ui.theme.fg("dim", command)}`,
		);
	}

	function setEnabled(next: boolean, ctx: ExtensionContext): void {
		enabled = next;
		persistConfig();
		if (!enabled) {
			clearReminder();
		}
		if (ctx.hasUI) {
			ctx.ui.notify(`revdiff post-edit reminders ${enabled ? "enabled" : "disabled"}`, "info");
		}
	}

	pi.registerFlag("revdiff-reminders", {
		description: "Enable post-edit revdiff reminders",
		type: "boolean",
		default: false,
	});

	pi.registerCommand("revdiff-reminders", {
		description: "Toggle/show post-edit revdiff reminders",
		handler: async (args, ctx) => {
			rememberCtx(ctx);
			const cmd = args.trim().toLowerCase();
			if (!cmd || cmd === "toggle") {
				setEnabled(!enabled, ctx);
				return;
			}
			if (cmd === "on" || cmd === "enable") {
				setEnabled(true, ctx);
				return;
			}
			if (cmd === "off" || cmd === "disable") {
				setEnabled(false, ctx);
				return;
			}
			if (cmd === "status") {
				const suffix = enabled ? `on (${resolveSuggestedCommand(ctx)})` : "off";
				ctx.ui.notify(`revdiff post-edit reminders: ${suffix}`, "info");
				return;
			}
			ctx.ui.notify("Usage: /revdiff-reminders [on|off|toggle|status]", "warning");
		},
	});

	pi.events.on("revdiff:launch", () => {
		clearReminder();
		editCallsInRun = 0;
	});

	pi.on("session_start", async (_event, ctx) => {
		rememberCtx(ctx);
		for (const entry of ctx.sessionManager.getBranch()) {
			if (entry.type === "custom" && entry.customType === CONFIG_TYPE) {
				const data = entry.data as ReminderConfig | undefined;
				if (typeof data?.enabled === "boolean") {
					enabled = data.enabled;
				}
			}
		}
		if (pi.getFlag("revdiff-reminders") === true) {
			enabled = true;
		}
		clearReminder();
	});

	pi.on("session_tree", async (_event, ctx) => {
		rememberCtx(ctx);
		clearReminder();
	});

	pi.on("agent_start", async (_event, ctx) => {
		rememberCtx(ctx);
		editCallsInRun = 0;
		clearReminder();
	});

	pi.on("tool_call", async (event) => {
		if (!enabled) {
			return;
		}
		if (event.toolName === "edit" || event.toolName === "write") {
			editCallsInRun++;
		}
	});

	pi.on("agent_end", async (_event, ctx) => {
		rememberCtx(ctx);
		if (!enabled || editCallsInRun === 0 || !ctx.hasUI) {
			return;
		}
		updateStatus(ctx);
		const command = resolveSuggestedCommand(ctx);
		const noun = editCallsInRun === 1 ? "edit" : "edits";
		ctx.ui.notify(`Agent made ${editCallsInRun} ${noun}. Consider ${command}`, "info");
	});
}

function getLastLaunch(ctx: ExtensionContext): LaunchMemory | undefined {
	let restored: LaunchMemory | undefined;
	for (const entry of ctx.sessionManager.getBranch()) {
		if (entry.type !== "custom" || entry.customType !== LAST_LAUNCH_TYPE) {
			continue;
		}
		const data = entry.data as LaunchMemory | { cleared?: true } | undefined;
		if (isLaunchMemory(data)) {
			restored = data;
		}
		if (data && typeof data === "object" && (data as { cleared?: true }).cleared === true) {
			restored = undefined;
		}
	}
	return restored;
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
