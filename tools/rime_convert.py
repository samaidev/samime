#!/usr/bin/env python3
"""
Rime 词库转换工具
将 Rime .dict.yaml 格式转换为 GoIME 的 `word pinyin freq` 文本格式

Rime .dict.yaml 格式示例:
    ---
    name: luna_pinyin
    version: '1.0'
    sort: by_weight
    ...
    # 注释行
    你好	ni hao	100
    中国	zhong guo	200
    单字	dan	zi	50  # Rime 用 Tab 分隔

用法:
    python3 rime_convert.py input.dict.yaml output.txt
    python3 rime_convert.py input.dict.yaml  # 输出到 stdout
    python3 rime_convert.py input.dict.yaml -o output.txt --default-freq 100

支持的特性:
    - 自动跳过 YAML header (---  ... 之间)
    - 跳过 # 注释行
    - 处理 Tab 或空格分隔
    - 拼音去除声调数字（如 ni3 -> ni）
    - 拼音合并空格（如 "ni hao" -> "nihao"）
    - 缺省频次填充
"""
import sys
import os
import re
import argparse

def strip_tone(s):
    """去除拼音声调数字：ni3 -> ni"""
    return re.sub(r'[1-5]', '', s)

def normalize_pinyin(py):
    """规范化拼音：去声调，去空格，转小写"""
    if not py:
        return ""
    py = strip_tone(py)
    py = re.sub(r'\s+', '', py)
    py = py.lower()
    return py

def is_chinese(s):
    return all('\u4e00' <= c <= '\u9fff' for c in s)

def convert(input_path, output_path=None, default_freq=1, min_freq=0):
    """转换 Rime dict.yaml 到 GoIME 格式"""
    out_entries = []
    skipped = 0

    in_header = False
    in_body = False

    with open(input_path, 'r', encoding='utf-8') as f:
        for line in f:
            line = line.rstrip('\n\r')

            if line.strip() == '---':
                in_header = True
                in_body = False
                continue
            if line.strip() == '...':
                in_header = False
                in_body = True
                continue

            stripped = line.strip()
            if not stripped or stripped.startswith('#'):
                continue

            if in_header:
                continue

            parts = re.split(r'\t+|\s{2,}', line)
            if len(parts) < 2:
                parts = line.split()
            if len(parts) < 2:
                skipped += 1
                continue

            word = parts[0]
            py_raw = parts[1]

            freq = default_freq
            if len(parts) >= 3:
                try:
                    freq = float(parts[2])
                except ValueError:
                    pass

            if freq < min_freq:
                skipped += 1
                continue

            if not is_chinese(word):
                skipped += 1
                continue

            py = normalize_pinyin(py_raw)
            if not py:
                skipped += 1
                continue

            out_entries.append((word, py, freq))

    seen = {}
    for w, p, f in out_entries:
        key = (w, p)
        if key not in seen or f > seen[key]:
            seen[key] = f

    if output_path:
        out = open(output_path, 'w', encoding='utf-8')
    else:
        out = sys.stdout

    try:
        out.write(f"# Converted from {os.path.basename(input_path)}\n")
        out.write(f"# Total entries: {len(seen)}\n")
        out.write(f"# Format: word pinyin freq\n")
        for (w, p), fr in sorted(seen.items(), key=lambda x: -x[1]):
            out.write(f"{w} {p} {fr}\n")
    finally:
        if output_path:
            out.close()

    return len(seen), skipped

def main():
    parser = argparse.ArgumentParser(description='Convert Rime .dict.yaml to GoIME format')
    parser.add_argument('input', help='Rime .dict.yaml file path')
    parser.add_argument('-o', '--output', help='Output file (default: stdout)', default=None)
    parser.add_argument('--default-freq', type=float, default=1, help='Default freq when missing (default: 1)')
    parser.add_argument('--min-freq', type=float, default=0, help='Skip entries with freq < min-freq (default: 0)')
    args = parser.parse_args()

    if not os.path.exists(args.input):
        print(f"Error: {args.input} not found", file=sys.stderr)
        sys.exit(1)

    n, skipped = convert(args.input, args.output, args.default_freq, args.min_freq)
    print(f"\n[OK] 转换 {n} 条, 跳过 {skipped} 条", file=sys.stderr)
    if args.output:
        print(f"输出文件: {args.output}", file=sys.stderr)

if __name__ == '__main__':
    main()
