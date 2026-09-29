# Loom Career Assistant — Memory Experiments: Lessons Learned

**Project:** Loom Career Assistant  
**Platform tested:** Linux, Oracle JDK 21.0.11 x64  
**Date:** 2026-09-23  
**Scope:** JVM heap/GC tuning, Linux process memory, and Skiko rendering mode

---

## 1. Executive summary

The experiments showed that Loom's high process memory is **not primarily caused by the application's Java object graph**.

Across the controlled runs, the live Java heap was only about **12–13 MiB**, while Linux attributed roughly **184–259 MiB PSS** to the process depending on JVM and rendering configuration.

The strongest findings were:

1. **Java heap usage is small.** The controlled normal run used about **13 MiB** of heap.
2. **G1 has disproportionate overhead for this workload.** In an early NMT snapshot, G1's `GC` category alone committed about **54.7 MiB**.
3. **Serial GC dramatically reduces GC infrastructure.** With `-XX:+UseSerialGC`, the NMT `GC` category fell to about **135 KiB** in the controlled low-memory runs.
4. **The low-memory JVM profile produces a real system-level saving.** In the controlled comparison it reduced:
   - RSS by about **34 MiB / 10.8%**
   - PSS by about **26 MiB / 9.9%**
5. **Software rendering reduced process memory further.** The two controlled software-rendering runs landed at about **184 MiB and 200 MiB PSS**, versus **233 MiB PSS** for the low-memory JVM with the default renderer.
6. **Renderer/native memory is now a larger optimization target than heap tuning.** The JVM itself remained almost unchanged between low-memory default rendering and low-memory software rendering, while Linux-visible memory changed substantially.
7. **NMT, RSS, and PSS answer different questions.** None should be used alone.
8. **A test name is not proof that the intended JVM flags were applied.** One supposed low-memory test was invalid because the app was still running G1 with NMT disabled.
9. **`MaxMetaspaceSize` is not justified by the current data.** Metaspace usage stayed around **37–38 MiB** across configurations and appears to represent real framework/class metadata.
10. **The best current configuration is an optional low-memory profile, not yet an unconditional production default.** It still needs realistic workload, responsiveness, and long-running stability testing.

---

## 2. What each memory metric means

### RSS — Resident Set Size

RSS is the amount of physical memory currently resident for the process.

It includes private pages and the process's full share of resident shared pages, so it can overstate how much memory should be attributed exclusively to Loom.

Use RSS when asking:

> "How many resident pages are associated with this process right now?"

### PSS — Proportional Set Size

PSS divides shared pages proportionally among the processes using them.

For comparing Loom configurations, **PSS is the best single Linux-level metric in these experiments** because it better approximates the memory attributable to the application.

Use PSS when asking:

> "How much physical memory should reasonably be charged to this process?"

### JVM Native Memory Tracking (NMT)

NMT reports memory tracked by the JVM in categories such as:

- Java heap
- GC
- metaspace/class metadata
- threads
- JIT/code cache
- symbols
- shared class space
- internal JVM allocations

NMT is extremely useful for explaining **JVM-side** memory, but it is **not a complete process-memory accounting system**. Native UI/graphics libraries, driver mappings, allocator behavior, file-backed pages, and other non-JVM components can cause RSS/PSS to be substantially higher than NMT committed memory.

### Reserved vs committed

A recurring source of confusion was the JVM's very large `reserved` values.

For example, the original G1 run reserved roughly **3.9 GiB** for the Java heap, but only about **60 MiB was committed**.

Reserved virtual address space is **not equivalent to physical RAM usage**.

For memory-footprint analysis, focus primarily on:

- heap **used**
- heap **committed**
- NMT **committed**
- Linux **PSS**
- Linux **RSS**

---

## 3. Experiment timeline

### 3.1 Initial normal JVM snapshot

The first inspection used the application's normal JVM configuration.

Observed approximately:

| Metric | Initial snapshot |
|---|---:|
| NMT committed | 194.9 MiB |
| RSS | 294.4 MiB |
| PSS | 231.4 MiB |
| Java heap committed | 60.0 MiB |
| Java heap used | 25.6 MiB |
| Metaspace used | ~34 MiB |
| GC committed | ~54.7 MiB |
| JVM threads | 44 |
| Thread committed | ~2.9 MiB |

This was the first strong indication that the heap itself was not responsible for the whole process footprint.

The especially important observation was:

```text
Java objects:       ~25.6 MiB
G1 GC structures:   ~54.7 MiB
```

The garbage collector's tracked infrastructure was consuming roughly twice as much committed memory as the Java objects themselves.

This motivated testing a collector designed for small heaps and simple workloads.

### 3.2 First manual Serial GC / 128 MiB experiment

The first manual low-memory configuration used:

```text
-Xms16m
-Xmx128m
-XX:+UseSerialGC
-XX:NativeMemoryTracking=summary
```

The JVM-side result was dramatic:

| JVM metric | Normal snapshot | Manual low-memory snapshot |
|---|---:|---:|
| NMT committed | ~194.9 MiB | ~122.6 MiB |
| Heap committed | 60.0 MiB | ~28.8 MiB |
| Heap used | 25.6 MiB | ~22.8 MiB |
| GC committed | ~54.7 MiB | ~0.12 MiB |

However, Linux PSS did **not** improve in that particular snapshot.

That initially looked contradictory, but the two runs were not sufficiently controlled: they were captured at different lifecycle/warm-up states, and file-backed memory differed.

**Lesson:** a large improvement in NMT committed memory does **not automatically imply an equal RSS/PSS improvement**. Uncontrolled snapshots are useful for diagnosis, but not for final A/B conclusions.

### 3.3 Controlled normal run — 60-second idle baseline

A later benchmark established a repeatable normal baseline after 60 seconds idle.

| Metric | Normal |
|---|---:|
| RSS | **316.9 MiB** |
| PSS | **258.6 MiB** |
| PSS anonymous | **210.4 MiB** |
| PSS file-backed | **47.3 MiB** |
| Private dirty | **229.4 MiB** |
| Heap committed | **48.0 MiB** |
| Heap used | **13.0 MiB** |
| Metaspace used | **37.3 MiB** |

This run did not have NMT enabled, so it could not be used for a controlled category-by-category NMT comparison.

**Lesson:** the live heap was only about **13 MiB** while PSS was about **259 MiB**. Loom's footprint cannot be explained as simply "the Java heap is too large."

### 3.4 Invalid "low-memory" run

One test was labelled `low-memory`, but inspection showed:

```text
Native memory tracking is not enabled
```

and:

```text
garbage-first heap
```

The heap was still 48 MiB committed, matching the normal controlled run.

This meant the intended low-memory JVM flags had **not actually reached the application JVM**.

**Lesson:** never trust the Gradle task name, shell script label, or launch command alone.

Every benchmark configuration should verify itself with:

```bash
jcmd <pid> VM.flags
jcmd <pid> GC.heap_info
```

For the low-memory profile we should explicitly see:

```text
-XX:+UseSerialGC
-XX:InitialHeapSize=16777216
-XX:MaxHeapSize=134217728
-XX:NativeMemoryTracking=summary
```

and `GC.heap_info` should report Serial GC generations rather than a `garbage-first heap`.

### 3.5 Valid controlled low-memory run

The corrected Gradle task, `:app:runLowMemory`, successfully applied:

```text
-Xms16m
-Xmx128m
-XX:+UseSerialGC
-XX:NativeMemoryTracking=summary
```

| Metric | Normal | Low-memory | Change |
|---|---:|---:|---:|
| RSS | 316.9 MiB | **282.7 MiB** | **-34.2 MiB (-10.8%)** |
| PSS | 258.6 MiB | **233.0 MiB** | **-25.6 MiB (-9.9%)** |
| PSS anonymous | 210.4 MiB | **181.1 MiB** | **-29.3 MiB (-13.9%)** |
| PSS file-backed | 47.3 MiB | **51.1 MiB** | +3.7 MiB |
| Private dirty | 229.4 MiB | **196.3 MiB** | **-33.0 MiB (-14.4%)** |
| Heap committed | 48.0 MiB | **31.0 MiB** | **-17.0 MiB (-35%)** |
| Heap used | 13.0 MiB | **~12.4 MiB** | essentially unchanged |
| Metaspace used | 37.3 MiB | **~37.4 MiB** | essentially unchanged |

The low-memory JVM also reported approximately:

| NMT category | Committed |
|---|---:|
| Total NMT committed | **121.7 MiB** |
| Java heap | **31.0 MiB** |
| Metaspace | **34.2 MiB** |
| Code | **18.5 MiB** |
| Threads | **2.3 MiB** |
| GC | **135 KiB** |
| NMT instrumentation | **2.1 MiB** |

This proved that the low-memory profile produced a **real Linux-level memory reduction**, not just a smaller JVM reservation.

Most importantly, heap **used** stayed nearly the same:

```text
Normal:      ~13.0 MiB
Low-memory:  ~12.4 MiB
```

The saving came from runtime overhead rather than the application suddenly holding less data.

### 3.6 Software rendering experiment

The next test combined the low-memory JVM with Skiko software rendering through the dedicated `:app:runLowMemorySoftware` path.

Two controlled software-rendering runs were captured.

#### Software run A

| Metric | Value |
|---|---:|
| RSS | **218.1 MiB** |
| PSS | **183.7 MiB** |
| PSS anonymous | **173.2 MiB** |
| PSS file-backed | **9.7 MiB** |
| NMT committed | **121.6 MiB** |

#### Software run B

| Metric | Value |
|---|---:|
| RSS | **225.7 MiB** |
| PSS | **199.9 MiB** |
| PSS anonymous | **180.6 MiB** |
| PSS file-backed | **18.5 MiB** |
| NMT committed | **122.7 MiB** |
| Heap used | **~12.5 MiB** |
| GC committed | **135 KiB** |

The JVM numbers remained effectively the same as the low-memory default-renderer run, while Linux-visible memory dropped substantially.

Using the latest software-rendering run:

| Metric | Low-memory default renderer | Low-memory software | Change |
|---|---:|---:|---:|
| RSS | 282.7 MiB | **225.7 MiB** | **-57.0 MiB (-20.2%)** |
| PSS | 233.0 MiB | **199.9 MiB** | **-33.1 MiB (-14.2%)** |
| PSS file-backed | 51.1 MiB | **18.5 MiB** | **-32.6 MiB** |
| NMT committed | 121.7 MiB | **122.7 MiB** | effectively unchanged |

Using the lower of the two software measurements, PSS reached **183.7 MiB**.

**Interpretation:** because NMT stayed almost constant while RSS/PSS changed sharply, the extra memory is not coming from Java heap or GC. The large reduction in `Pss_File` strongly suggests that the hardware/default rendering path brings in a meaningful amount of graphics-driver/library/file-backed memory.

This is evidence about the **process footprint**, not yet proof that software rendering should become the universal default.

---

## 4. Consolidated controlled results

| Configuration | RSS | PSS | Heap used | Heap committed | Notes |
|---|---:|---:|---:|---:|---|
| Normal JVM + default renderer | **316.9 MiB** | **258.6 MiB** | ~13.0 MiB | 48.0 MiB | G1 |
| Low-memory JVM + default renderer | **282.7 MiB** | **233.0 MiB** | ~12.4 MiB | 31.0 MiB | Serial GC |
| Low-memory JVM + software, run A | **218.1 MiB** | **183.7 MiB** | ~12.4 MiB | ~31 MiB | Serial GC |
| Low-memory JVM + software, run B | **225.7 MiB** | **199.9 MiB** | ~12.5 MiB | ~31 MiB | Serial GC |

From the normal controlled run to the latest software-rendering run:

- RSS: **316.9 → 225.7 MiB**
  - saving: **~91 MiB**
  - reduction: **~28.8%**
- PSS: **258.6 → 199.9 MiB**
  - saving: **~58.7 MiB**
  - reduction: **~22.7%**

The best software snapshot reached approximately **183.7 MiB PSS**, but two runs are not enough to treat that as the stable expected value.

---

## 5. Core lessons learned

### 5.1 The application's live Java object graph is small

A ~300 MiB process does **not** imply a ~300 MiB heap.

In the controlled tests Loom used only about **12–13 MiB of live Java heap** after the idle period.

The majority of the process footprint comes from runtime, native, graphics, mapped libraries, and other infrastructure.

### 5.2 `-Xmx` is not a process-memory limit

Setting:

```text
-Xmx128m
```

does not mean Loom will become a 128 MiB process.

`-Xmx` limits the Java heap, not metaspace, JIT code cache, thread stacks, JVM native structures, Skia/Skiko allocations, graphics-driver mappings, shared/native libraries, or other native allocations.

### 5.3 Serial GC fits this small-heap workload much better than G1

The clearest JVM-side finding was the GC category.

An early G1 NMT snapshot showed roughly:

```text
GC committed ≈ 54.7 MiB
```

Serial GC reduced the corresponding category to about:

```text
GC committed ≈ 135 KiB
```

For a desktop CRUD application with only ~12–25 MiB of live objects during these tests, G1's infrastructure was disproportionately expensive.

This does **not** prove Serial GC is always superior. It means Serial GC is a strong candidate for Loom's low-memory workload and deserves continued performance/stability validation.

### 5.4 PSS is more useful than RSS for configuration comparisons

RSS remains useful, but PSS better estimates how much RAM should be attributed to Loom after shared mappings are accounted for.

For future comparisons, primary metrics should be:

1. **PSS**
2. RSS
3. PSS anonymous/file-backed breakdown
4. NMT committed categories
5. live/committed heap

### 5.5 NMT is diagnostic, not a replacement for `/proc`

The experiments repeatedly showed a large gap between NMT committed memory and Linux PSS.

That gap is expected because NMT does not fully account for every library, rendering resource, driver mapping, and native allocation visible at the process level.

Use both:

```bash
jcmd <pid> VM.native_memory summary
cat /proc/<pid>/smaps_rollup
```

### 5.6 Software rendering changes memory outside the JVM

The software-rendering runs had almost the same heap, metaspace, GC, threads, code cache, and total NMT committed, yet substantially lower RSS/PSS.

That is one of the strongest findings of the entire investigation.

The renderer affects the **native/process-level footprint**, particularly file-backed memory, far more than it affects JVM memory.

### 5.7 Run-to-run variance is real

The two software-rendering runs produced:

```text
PSS ≈ 183.7 MiB
PSS ≈ 199.9 MiB
```

That is roughly a 16 MiB spread.

A single process snapshot should therefore not be treated as a benchmark result.

Future conclusions should use multiple repetitions and a robust summary such as the median.

### 5.8 Metaspace is not currently an obvious optimization target

Metaspace remained around **37–38 MiB used** across the controlled configurations.

That suggests the memory is associated with classes/frameworks Loom actually loads rather than merely excessive JVM preallocation.

There is currently no evidence supporting a `MaxMetaspaceSize` cap.

### 5.9 NMT itself has measurable overhead

In the low-memory runs, NMT instrumentation consumed about **2.1 MiB**.

Therefore:

```text
-XX:NativeMemoryTracking=summary
```

is appropriate for profiling and experiments, but it does not need to be permanently enabled in a normal production launch unless its diagnostics are desired.

### 5.10 Validate the actual child JVM

Gradle can launch another JVM for the application, and environment variables can also affect more JVMs than intended.

A correct benchmark must validate the **actual Loom JVM**, not assume flags propagated correctly.

Required validation:

```bash
jcmd <pid> VM.flags
jcmd <pid> GC.heap_info
```

---

## 6. Recommended configuration status

### Normal launch

Preserve the current normal launch behavior while experimentation continues.

Do not silently replace the default configuration solely from idle-memory results.

### Low-memory JVM profile

Keep the dedicated low-memory mode:

```text
-Xms16m
-Xmx128m
-XX:+UseSerialGC
```

Enable NMT only for profiling:

```text
-XX:NativeMemoryTracking=summary
```

This profile has already demonstrated roughly a **10% PSS reduction** in a controlled idle comparison.

Before treating `-Xmx128m` as production-safe, test realistic worst-case workspace sizes and extended usage.

### Low-memory + software rendering

Keep a separate software-rendered low-memory mode.

Current evidence indicates a substantial additional memory saving, but it needs CPU and UI-performance validation before being selected automatically or made the universal default.

---

## 7. What we should not conclude yet

The experiments do **not** yet prove that:

- 128 MiB is enough for every realistic Loom workload.
- Serial GC has no latency or responsiveness downside during active use.
- software rendering is always preferable to GPU rendering.
- the lowest observed PSS value is the normal steady-state footprint.
- the remaining native memory is all Skia or all GPU-driver memory.
- the current application has no long-running memory leaks.
- idle memory behavior represents large-workspace or heavy-navigation behavior.

Those require separate experiments.

---

## 8. Recommended next experiments

### 8.1 Repeat each configuration multiple times

Run at least 5 repetitions of:

```text
normal
low-memory
low-memory-software
```

with the same startup procedure, workspace/data set, renderer, window size, navigation path, and 60-second idle period.

Report median PSS, median RSS, minimum/maximum, and ideally standard deviation or interquartile range.

### 8.2 Add an active UI benchmark

Idle memory alone is not enough to choose a renderer.

Exercise fast scrolling, repeated screen changes, dialogs, window resize, list-heavy workspaces, text editing, animations, CRUD operations, and network-enabled screens if applicable.

Measure CPU usage, responsiveness, frame consistency/jank, PSS/RSS after the workload, and GC activity.

### 8.3 Test realistic maximum workspaces

The 128 MiB max heap should be tested against large job/application histories, many workspace records, long descriptions, large generated CV/document data, repeated navigation, imports, and extended editing sessions.

### 8.4 Long-running stability test

Leave Loom running for hours while periodically exercising normal operations.

Track PSS/RSS over time to distinguish stable caches, expected warm-up, allocator retention, and genuine memory growth/leaks.

### 8.5 Inspect memory mappings directly

Because software rendering changed `Pss_File` so much, inspect individual mappings in:

```text
/proc/<pid>/smaps
```

or equivalent mapping summaries.

The goal is to identify which libraries/mappings account for the default renderer's additional file-backed PSS.

### 8.6 Audit loaded dependencies after renderer analysis

Only after the larger native/rendering costs are understood should significant effort go into smaller framework costs.

Potential questions:

- Are all Compose modules actually required?
- Is the extended Material icon artifact necessary?
- Are networking modules initialized eagerly?
- Can some services be lazy-loaded?
- Are unused runtime dependencies being pulled into the desktop application?

---

## 9. Benchmark checklist

For every future memory benchmark:

```text
[ ] Same application revision
[ ] Same JDK
[ ] Same input/workspace data
[ ] Same window/render state
[ ] Same idle/warm-up duration
[ ] Verify VM.flags
[ ] Verify collector with GC.heap_info
[ ] Capture VM.native_memory summary when NMT is enabled
[ ] Capture /proc/<pid>/smaps_rollup
[ ] Record RSS and PSS
[ ] Record PSS_Anon and PSS_File
[ ] Record heap used and committed
[ ] Record metaspace
[ ] Record thread count
[ ] Repeat multiple times
[ ] Compare medians rather than isolated best runs
```

---

## 10. Current working model of Loom's memory footprint

```text
Loom process
│
├── Application Java objects              ~12–13 MiB in controlled idle tests
├── Java heap headroom                     additional committed heap
├── Metaspace / class metadata             ~37 MiB used
├── JIT / code cache                       ~18 MiB committed in low-memory runs
├── JVM symbols / shared class data        additional JVM memory
├── Thread stacks / JVM internals          a few MiB committed
├── GC infrastructure
│   ├── G1                                 expensive in the early normal snapshot
│   └── Serial GC                          ~135 KiB tracked in controlled tests
├── Skia / Skiko / AWT native state
├── graphics-driver / renderer mappings
├── shared native libraries
└── other native / allocator / mapped memory
```

The major conceptual shift is:

> Loom is not a 250–300 MiB Java object graph. It is a small Java object graph running inside a comparatively expensive desktop JVM + Compose/Skiko/native graphics stack.

That distinction tells us where optimization effort is likely to produce real returns.

---

## 11. Current engineering conclusions

The experiments justify the following working decisions:

1. **Keep normal behavior available.**
2. **Keep `runLowMemory` as a dedicated test/low-memory profile.**
3. **Use Serial GC in that profile.**
4. **Keep `-Xms16m` and `-Xmx128m` as the current experimental bounds.**
5. **Do not add `MaxMetaspaceSize` based on current evidence.**
6. **Use NMT when profiling, not necessarily on every production launch.**
7. **Keep `runLowMemorySoftware` as a distinct mode while measuring CPU/UX trade-offs.**
8. **Use PSS as the primary process-footprint comparison metric.**
9. **Always verify the actual JVM flags before accepting benchmark results.**
10. **Shift the next optimization effort toward rendering/native mappings rather than further heap micro-tuning.**

---

## 12. Final takeaway

The investigation started with a reasonable complaint:

> Why does a simple CRUD desktop application occupy roughly 300 MiB of RAM?

The data now gives a much more precise answer.

Loom's business data and Java objects are not consuming hundreds of megabytes. In the controlled tests, the live heap was only about **12–13 MiB**.

The larger footprint comes from the complete desktop runtime: JVM metadata and JIT infrastructure, the garbage collector, Compose Desktop, Skiko/Skia, graphics/native libraries, mappings, and shared process infrastructure.

Two changes produced measurable improvements without changing the application model:

```text
G1 → Serial GC
default renderer → software renderer
```

The low-memory JVM alone reduced controlled PSS by about **10%**. Adding software rendering reduced it further, with the observed software runs landing around **184–200 MiB PSS**.

The next objective should therefore not be "make the heap even smaller."

It should be:

> **Measure and reduce the native/rendering floor while preserving responsiveness, correctness, and realistic workload headroom.**
