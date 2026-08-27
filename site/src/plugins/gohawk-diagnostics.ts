import {
	ExpressiveCodeAnnotation,
	type AnnotationRenderOptions,
	type ExpressiveCodeInlineRange,
	type ExpressiveCodeLine,
	type ExpressiveCodePlugin,
} from '@expressive-code/core';
import { addClassName, h, setProperty } from '@expressive-code/core/hast';

type Diagnostic = {
	message: string;
	startLine: number;
	startColumn: number;
	endLine: number;
	endColumn: number;
};

type DiagnosticRange = {
	line: ExpressiveCodeLine;
	columnStart: number;
	columnEnd: number;
	messages: string[];
};

class GohawkDiagnosticAnnotation extends ExpressiveCodeAnnotation {
	readonly messages: string[];

	constructor(inlineRange: ExpressiveCodeInlineRange, messages: string[]) {
		super({ inlineRange, renderPhase: 'latest' });
		this.messages = messages;
	}

	render({ nodesToTransform }: AnnotationRenderOptions) {
		return nodesToTransform.map((node) => {
			const marked = h('span', node);
			addClassName(marked, 'gohawk-diagnostic');
			setProperty(marked, 'data-gohawk-message', this.messages.join('\n'));
			setProperty(marked, 'tabIndex', '0');
			return marked;
		});
	}
}

function isDiagnostic(value: unknown): value is Diagnostic {
	if (typeof value !== 'object' || value === null) return false;

	const diagnostic = value as Record<string, unknown>;
	return (
		typeof diagnostic.message === 'string' &&
		Number.isInteger(diagnostic.startLine) &&
		Number.isInteger(diagnostic.startColumn) &&
		Number.isInteger(diagnostic.endLine) &&
		Number.isInteger(diagnostic.endColumn)
	);
}

function decodedDiagnostics(value: string): Diagnostic[] {
	try {
		const parsed: unknown = JSON.parse(Buffer.from(value, 'base64url').toString('utf8'));
		return Array.isArray(parsed) ? parsed.filter(isDiagnostic) : [];
	} catch {
		return [];
	}
}

export function pluginGohawkDiagnostics(): ExpressiveCodePlugin {
	return {
		name: 'gohawk diagnostics v2',
		hooks: {
			annotateCode: ({ codeBlock }) => {
				const encoded = codeBlock.metaOptions.getString('gohawk');
				if (!encoded) return;

				const ranges = new Map<string, DiagnosticRange>();
				for (const diagnostic of decodedDiagnostics(encoded)) {
					for (let lineIndex = diagnostic.startLine; lineIndex <= diagnostic.endLine; lineIndex++) {
						const line = codeBlock.getLine(lineIndex);
						if (!line) continue;
						const columnStart = lineIndex === diagnostic.startLine ? diagnostic.startColumn : 0;
						const columnEnd = lineIndex === diagnostic.endLine ? diagnostic.endColumn : line.text.length;
						if (columnEnd <= columnStart) continue;
						const key = `${lineIndex}:${columnStart}:${columnEnd}`;
						const range = ranges.get(key) ?? { line, columnStart, columnEnd, messages: [] };
						range.messages.push(diagnostic.message);
						ranges.set(key, range);
					}
				}

				for (const { line, columnStart, columnEnd, messages } of ranges.values()) {
					line.addAnnotation(
						new GohawkDiagnosticAnnotation({ columnStart, columnEnd }, messages),
					);
				}
			},
		},
	};
}
