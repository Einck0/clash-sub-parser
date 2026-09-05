import re
import sys

def migrate_style():
    with open('src/style.css', 'r', encoding='utf-8') as f:
        content = f.read()

    # Replacements for legacy tokens
    replacements = [
        (r'var\(--brand-2\)', 'var(--color-brand-accent, #06b6d4)'),
        (r'var\(--brand-warm\)', 'var(--color-status-warning, #f59e0b)'),
        (r'var\(--brand\)', 'var(--color-accent, #3b82f6)'),
        (r'var\(--ink-soft\)', 'var(--color-text-muted, #94a3b8)'),
        (r'var\(--ink\)', 'var(--color-text-main, #f8fafc)'),
        (r'var\(--surface-3\)', 'var(--color-surface-active, #334155)'),
        (r'var\(--surface-2\)', 'var(--color-surface-hover, #1e293b)'),
        (r'var\(--surface\)', 'var(--color-surface-base, #0f172a)'),
        (r'var\(--bg-0\)', 'var(--color-canvas, #090d16)'),
        (r'var\(--bg-1\)', 'var(--color-surface-base, #0f172a)'),
        (r'var\(--border-strong\)', 'var(--color-border-strong, #334155)'),
        (r'var\(--border\)', 'var(--color-border, #1e293b)'),
        (r'var\(--danger\)', 'var(--color-status-danger, #ef4444)'),
        (r'var\(--ok\)', 'var(--color-status-success, #10b981)'),
        (r'var\(--warning\)', 'var(--color-status-warning, #f59e0b)'),
    ]

    for pat, repl in replacements:
        content = re.sub(pat, repl, content)

    # Remove any remaining raw .modal-backdrop and .modal top-level rules
    # .modal-backdrop { ... }
    content = re.sub(r'^\.modal-backdrop\s*\{[^}]*\}', '/* legacy .modal-backdrop removed */', content, flags=re.MULTILINE)
    # .modal { ... }
    content = re.sub(r'^\.modal\s*\{[^}]*\}', '/* legacy .modal removed */', content, flags=re.MULTILINE)
    # [data-theme="dark"] .modal { ... }
    content = re.sub(r'^\[data-theme="dark"\]\s+\.modal\s*\{[^}]*\}', '/* legacy [data-theme="dark"] .modal removed */', content, flags=re.MULTILINE)
    # , .modal
    content = re.sub(r',\s*\n\.modal\b', '', content)

    with open('src/style.css', 'w', encoding='utf-8') as f:
        f.write(content)
    print("Migration complete!")

if __name__ == '__main__':
    migrate_style()
