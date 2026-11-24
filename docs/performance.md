# Performance and profiling

Measure frame building, software rendering, and native presentation separately. A headless benchmark cannot tell you the actual FPS of a window: it does not include GPU work, a display server, or vsync.

## Reproduce the measurements

The end-to-end editor benchmarks now live in [Flint](https://github.com/FlameInTheDark/flint). From the Flint repository root:

```sh
go test ./internal/workspace -run '^$' -bench '^BenchmarkSoftwarePaint$' -benchmem -count=3 -benchtime=500ms
go test ./internal/workspace -run '^$' -bench '^BenchmarkSoftwarePaint$' -benchtime=3s -cpuprofile=software.cpu -memprofile=software.mem -o=software.test
go tool pprof -top software.cpu
go tool pprof -top -sample_index=alloc_space software.mem
go tool pprof -top -sample_index=alloc_objects software.mem
```

`BenchmarkSoftwarePaint` builds and rasterizes an editor frame, including a headless RGBA snapshot. Headless snapshots remain valid across draws; native backends can instead reuse pixels that app code never sees.

To isolate untransformed monochrome glyphs and solid rounded quads, run from the **go-gpui** repository root:

```sh
go test ./render -run '^$' -bench '^BenchmarkSoftware(Glyphs|RoundedQuads)$' -benchmem -count=3 -benchtime=350ms
```

To isolate frame building and input handling from rasterization, run from the **Flint** repository root:

```sh
go run ./bench -big 10000 -frames 1500 -cpuprofile frame-build.cpu
go tool pprof -top frame-build.cpu
```

`-cpuprofile` and `-png` are optional and write to paths you supply. For comparison runs use the same Go version, fonts, machine, and benchmark settings. Record allocation volume (`B/op`) and allocation count (`allocs/op`) alongside frame time.

## Software renderer results

Median of three headless editor benchmark runs on Windows/amd64, Intel Core Ultra 9 275HX, Go 1.27.1:

| Per frame | Original | After framebuffer and path reuse | After glyph, quad and layout changes |
| --- | ---: | ---: | ---: |
| Time | 17.45 ms | 12.12 ms | 8.55 ms |
| Bytes allocated | 17.34 MB | 5.66 MB | 5.35 MB |
| Allocations | 167,691 | 7,152 | 7,026 |

The first profile showed most allocated bytes coming from frame buffers and most allocations by count from per-sample path crossings. The software backends now reuse the float framebuffer while the size is unchanged. X11 and Wayland also reuse their RGBA output, and X11 keeps the packed upload buffer for expose redraws. Software path filling solves curve crossings once per subrow rather than once per sample, preserving both fill rules and the original antialiasing sample positions. Windows GDI already reused its full-frame buffers; it shares the path improvement. The table measures the headless workload, not native presentation speed.

The next pass measured the two software drawing operations separately. For narrow, untransformed glyphs, the renderer computes horizontal atlas coordinates once per glyph and picks the two bilinear texture rows once per output row. A solid rounded quad skips shader math in the interior strip where a corner cannot affect the pixel; edges still use the general shader.

| Isolated benchmark | Before | After |
| --- | ---: | ---: |
| 1,200 synthetic monochrome glyphs | 3.57 ms | 1.75 ms |
| 24 opaque borderless rounded quads | 5.17 ms | 1.18 ms |

Windows now reuse layout node and map storage across frames, clearing cached sizes and releasing old element references before rebuilding. The editor caches token geometry in a per-buffer ring sized to the visible minimap plus nearby rows. It invalidates on edits, language or palette changes, and changes in horizontal scale. The large-buffer steady-state frame-build run measured about 1.02 ms/frame, versus 1.18 ms before these changes. Those two frame-build figures are single-run observations, not standalone minimap or layout benchmarks.

## Where to look next

1. **Software drawing:** general quad shading and RGBA export remain substantial in the headless profile. Distinguish border, gradient, and translucent cases before adding more fast paths; headless snapshot export and native blits are different workloads.
2. **Frame building:** benchmark idle frames, scroll, edits, and resize separately. In particular, measure whether invalidating only changed minimap rows helps rapid editing, and whether large views retain too much layout storage after shrinking.
3. **Scene ownership:** frame construction allocates a new scene and grows primitive lists every redraw. A reusable staging buffer or scene pool may reduce allocation pressure on both renderers, but scenes can be retained by callers and by delayed presentation. Define their ownership before reusing backing arrays.
4. **Native latency:** measure Direct3D instance building and upload, X11 frame copies, and Wayland buffer commits separately. Compare input-to-present latency and missed frames, not just average headless frame time.
