#!/usr/bin/env python3
"""Export a clean pinned source snapshot; instrument only lifecycle/evidence seams."""
import argparse, hashlib, pathlib, subprocess
PIN = '3c320fd58aff5235e16218c050ec5b8ce587e233'
p = argparse.ArgumentParser()
p.add_argument('--upstream', required=True)
p.add_argument('--workspace', required=True)
a = p.parse_args()
root = pathlib.Path(__file__).resolve().parents[2]
workspace = pathlib.Path(a.workspace).resolve()
if workspace.exists():
    raise SystemExit('workspace must not exist: preserve cached and existing workspaces')
workspace.mkdir(mode=0o700, parents=True)
archive = subprocess.run(['git', '-C', a.upstream, 'archive', PIN], check=True, stdout=subprocess.PIPE).stdout
subprocess.run(['tar', '-x', '-C', str(workspace)], input=archive, check=True)
(workspace/'SOURCE.sha256').write_text(hashlib.sha256(archive).hexdigest()+'  git-archive '+PIN+'\n')
# Reuse the SAME response reservation/settlement implementation in the isolated
# upstream module. No GPL engine code is copied back to the CSP product.
pool = (root/'internal/probe/platform/pool.go').read_text().replace('package platform', 'package check', 1)
(workspace/'check/reference_budget.go').write_text(pool)
source = workspace/'check/check.go'
s = source.read_text()
def replace(old, new):
    global s
    if s.count(old)!=1: raise SystemExit('instrumentation anchor drift: '+old)
    s=s.replace(old,new,1)
replace('ctx, cancel := context.WithCancel(context.Background())\n\tdefer installPhaseCancel(cancel)()', 'ctx, cancel := context.WithCancel(referenceContext)\n\tdefer installPhaseCancel(cancel)()')
replace('func (pc *ProxyChecker) checkAlive(proxy map[string]any) *aliveResult {', 'func (pc *ProxyChecker) checkAlive(proxy map[string]any) (out *aliveResult) {\n defer func(){ referenceFinish(proxy, "baseline", out != nil, nil) }()')
replace('httpClient := CreateClient(proxy)', 'httpClient := referenceClient(proxy, "baseline")')
replace('func (pc *ProxyChecker) checkSpeed(r Result, speedTestURL string) *Result {', 'func (pc *ProxyChecker) checkSpeed(r Result, speedTestURL string) (out *Result) {\n defer func(){ referenceFinish(r.Proxy, "speed", out != nil, out) }()')
replace('httpClient := CreateClient(r.Proxy)', 'httpClient := referenceClient(r.Proxy, "speed")')
replace('func (pc *ProxyChecker) checkMedia(a aliveResult) *Result {', 'func (pc *ProxyChecker) checkMedia(a aliveResult) (out *Result) {\n defer func(){ referenceFinish(a.Proxy, "media", out != nil, out) }()')
replace('httpClient := CreateClient(a.Proxy)', 'httpClient := referenceClient(a.Proxy, "media")')
replace('alive, err := platform.CheckAlive(httpClient.Client)', 'alive, err := platform.CheckAlive(httpClient.Client)\n referenceError(proxy, "baseline", "baseline", err)')
replace('speed, _, err := platform.CheckSpeed(httpClient.Client, Bucket, httpClient.BytesRead, speedTestURL)', 'speed, _, err := platform.CheckSpeed(httpClient.Client, Bucket, httpClient.BytesRead, speedTestURL)\n referenceError(r.Proxy, "speed", "speed", err)')
# Preserve returned errors at the real callsites, including non-collector paths.
for method, field in [('Youtube','region'),('Gemini','region'),('Claude','region'),('Spotify','region'),('TikTok','region')]:
    old=f'if {field}, _ := platform.Check{method}(mediaClient); {field} != "" {{'
    new=f'{field}, platformErr := platform.Check{method}(mediaClient)\n referenceError(a.Proxy,"media","{method.lower()}",platformErr)\n if {field} != "" {{'
    replace(old,new)
for method, var in [('Netflix','nf'),('Disney','d')]:
    replace(f'{var}, _ := platform.Check{method}(mediaClient)',f'{var}, platformErr := platform.Check{method}(mediaClient)\n referenceError(a.Proxy,"media","{method.lower()}",platformErr)')
replace('risk, err := platform.CheckIPRisk(mediaClient, ip)', 'risk, err := platform.CheckIPRisk(mediaClient, ip)\n referenceError(a.Proxy,"media","iprisk",err)')
# Comparative media clones must retain the aligned redirect policy; native remains default.
replace('// 并行检测所有平台','if referenceInputState.Mode=="stage" {mediaClient.CheckRedirect=httpClient.Client.CheckRedirect}\n // 并行检测所有平台')
source.write_text(s)
# The upstream alive contract already is any2xx. Instrument its formerly unread
# body for comparable baseline accounting, without replacing its verdict logic.
alive=workspace/'check/platform/alive.go'
s=alive.read_text().replace('"net/http"','"net/http"\n "io"')
s=s.replace('defer resp.Body.Close()', 'defer resp.Body.Close()\n if _, err := io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10)); err != nil { return false, err }')
alive.write_text(s)
(workspace/'check/reference_entry.go').write_text((root/'tools/probe-comparison/reference_entry.go.txt').read_text())
(workspace/'check/reference_fixture_test.go').write_text((root/'tools/probe-comparison/reference_fixture_test.go.txt').read_text())
(workspace/'cmd/reference').mkdir(parents=True)
(workspace/'cmd/reference/main.go').write_text('package main\nimport ("os"; "github.com/beck-8/subs-check/check")\nfunc main(){if check.ReferenceMain(os.Stdin,os.Stdout)!=nil {os.Exit(1)}}\n')
subprocess.run(['gofmt','-w',str(workspace/'check/reference_budget.go'),str(workspace/'check/reference_entry.go'),str(workspace/'check/reference_fixture_test.go'),str(source),str(alive),str(workspace/'cmd/reference/main.go')],check=True)
# A stable provenance diff against the cached commit, not against untracked files.
for relative in ['check/check.go','check/platform/alive.go']:
    base=subprocess.run(['git','-C',a.upstream,'show',PIN+':'+relative],stdout=subprocess.PIPE,check=True).stdout
    (workspace/(relative.replace('/','_')+'.base')).write_bytes(base)
    diff=subprocess.run(['diff','-u',str(workspace/(relative.replace('/','_')+'.base')),str(workspace/relative)],stdout=subprocess.PIPE)
    (workspace/(relative.replace('/','_')+'.patch')).write_bytes(diff.stdout)
# Separate comparative control only: primary go.mod/go.sum are never rewritten.
mod=(workspace/'go.mod').read_text()
if 'github.com/metacubex/mihomo v1.19.31' not in mod: raise SystemExit('unexpected native Mihomo pin')
(workspace/'samecore.mod').write_text(mod.replace('github.com/metacubex/mihomo v1.19.31','github.com/metacubex/mihomo v1.19.32'))
(workspace/'samecore.sum').write_text((workspace/'go.sum').read_text()+'\n'+(root/'go.sum').read_text())
for relative in ['go.mod','go.sum']:
    other=workspace/('samecore.mod' if relative=='go.mod' else 'samecore.sum')
    diff=subprocess.run(['diff','-u',str(workspace/relative),str(other)],stdout=subprocess.PIPE)
    (workspace/(relative+'.samecore.patch')).write_bytes(diff.stdout)
print(workspace)
