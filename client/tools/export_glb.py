"""tools/export_glb.py - bundled Blendkit-client recipe.

Re-exports the active scene to .glb. Invoked via
POST /run_blender_script with `script_id="export_glb"`.

Recipe ABI (every script in tools/ follows this):
    sys.argv = [..., "--", <params.json>]
    params.json keys (this script):
        output_path    : str   (required) - destination .glb
        yup            : bool  (default True)
        draco          : bool  (default False)
        export_apply   : bool  (default True)
        texture_max_px : int   (default 0)      - downscale every image whose
                                                  longest side exceeds this to
                                                  fit it; 0 disables the cap.
        image_format   : str   (default "AUTO") - "WEBP" / "JPEG" / "PNG" /
                                                  "AUTO". Re-encode embedded
                                                  textures; "AUTO" keeps the
                                                  source format.
        image_quality  : int   (default 85)     - lossy quality for WEBP/JPEG.

Why this matters (Rhino host): Blendkit assets without a server-generated
resolution variant ship the ORIGINAL blend with up to 8192x8192 uncompressed
PNG textures packed in. Exported verbatim that produces 100-270 MB GLBs whose
import freezes Rhino's (synchronous, UI-thread) glTF importer for minutes.
Downscaling to the requested resolution + WEBP re-encoding takes those same
assets to well under 1 MB and imports in a fraction of a second. The mesh is
almost never the problem; the textures are.
"""

import json
import sys

import bpy


def ensure_gltf_addon():
    """Make sure ``bpy.ops.export_scene.gltf`` is callable.

    The glTF I/O addon ships with Blender but isn't always enabled by
    default — when it isn't, ``export_scene.gltf`` raises
    ``AttributeError: ... has no attribute 'gltf'`` and the export
    fails before we get a useful error. Worse, the module name has
    moved over time:

      * Blender 3.x / 4.0 / 4.1: classic addon ``io_scene_gltf2``.
      * Blender 4.2+: bundled extensions repo. Module is
        ``bl_ext.<repo_id>.io_scene_gltf2`` where ``repo_id`` varies
        per install (``system``, ``user_default``, ``blender_org``).

    A hand-maintained list of names will rot, so we discover candidates
    dynamically with ``addon_utils.modules()`` and pick anything whose
    bl_info advertises a glTF importer/exporter — then fall back to a
    short list of known names if the scan didn't find any (e.g. the
    extensions repo wasn't refreshed yet).

    Enabling goes through ``addon_utils.enable(default_set=True)`` so
    the change is persistent (saved into userprefs after we
    ``save_userpref`` — non-fatal if that step can't write to the prefs
    file in a sandboxed background run).
    """
    if hasattr(bpy.ops.export_scene, "gltf"):
        return  # already available — nothing to do

    try:
        import addon_utils
    except Exception as exc:
        # No addon_utils means we're inside something that's not real
        # Blender — bail with a clear error instead of carrying on.
        raise RuntimeError(
            f"export_glb: addon_utils unavailable ({exc!r}); cannot enable glTF addon.",
        ) from exc

    # 1) Discover by scanning installed modules. Survives the
    #    addon→extension rename without us tracking module paths.
    candidates = []
    seen = set()
    try:
        for mod in addon_utils.modules(refresh=False):
            modname = getattr(mod, "__name__", None)
            if not modname or modname in seen:
                continue
            try:
                info = addon_utils.module_bl_info(mod) or {}
            except Exception:
                info = {}
            name = (info.get("name") or "").lower()
            cat = (info.get("category") or "").lower()
            # bl_info names are stable: "glTF 2.0 format" for both the
            # classic addon and the extension. Match on substring so
            # any future rename (eg. "glTF 3.0") still picks it up.
            if "gltf" in name or ("import-export" in cat and "gltf" in modname.lower()):
                candidates.append(modname)
                seen.add(modname)
    except Exception as scan_exc:
        print(
            f"export_glb: addon_utils scan failed ({scan_exc!r}); falling back to known names.",
            file=sys.stderr,
        )

    # 2) Always also try well-known module names — covers the case
    #    where the extensions repo index hasn't been built yet on a
    #    fresh Blender install.
    for name in (
        "io_scene_gltf2",
        "bl_ext.system.io_scene_gltf2",
        "bl_ext.user_default.io_scene_gltf2",
        "bl_ext.blender_org.io_scene_gltf2",
    ):
        if name not in seen:
            candidates.append(name)
            seen.add(name)

    last_err = None
    for module in candidates:
        try:
            mod = addon_utils.enable(module, default_set=True, persistent=True)
        except Exception as exc:
            last_err = exc
            continue
        if mod is None:
            # enable() returns None when the module wasn't found.
            continue
        if hasattr(bpy.ops.export_scene, "gltf"):
            print(f"export_glb: enabled glTF addon ({module})")
            try:
                bpy.ops.wm.save_userpref()
            except Exception as save_exc:
                # Headless / read-only-prefs runs can't persist; the
                # in-process enable is still enough for THIS export.
                print(
                    f"export_glb: enabled but couldn't save prefs ({save_exc!r})",
                    file=sys.stderr,
                )
            return

    raise RuntimeError(
        f"export_glb: could not enable glTF addon. tried_candidates={candidates} last_err={last_err!r}",
    )


argv = sys.argv
if "--" not in argv:
    print("export_glb: missing -- separator", file=sys.stderr)
    sys.exit(2)
argv = argv[argv.index("--") + 1 :]
if not argv:
    print("export_glb: no params.json path given after --", file=sys.stderr)
    sys.exit(2)

with open(argv[-1], encoding="utf-8") as fh:
    params = json.load(fh)


def downscale_images(cap):
    """Shrink every image whose longest side exceeds ``cap`` px so it fits.

    Preserves aspect ratio. Returns a list of "WxH->wxh" strings for logging.
    ``cap <= 0`` is a no-op.

    ``img.size`` is read straight off the datablock (available even for a
    packed image that has not been loaded into memory yet); ``img.scale()``
    forces the load + resize. We never save the .blend, so this only affects
    the in-memory scene we are about to export — the cached source file is
    untouched.
    """
    if cap <= 0:
        return []
    scaled = []
    for img in bpy.data.images:
        # Skip generated/render-result/viewer images — only real textures.
        if getattr(img, "type", "IMAGE") != "IMAGE":
            continue
        try:
            w, h = img.size
        except Exception:  # noqa: S112
            continue
        if not w or not h:
            continue
        longest = max(w, h)
        if longest <= cap:
            continue
        nw = max(1, int(round(w * cap / longest)))  # noqa: RUF046
        nh = max(1, int(round(h * cap / longest)))  # noqa: RUF046
        try:
            img.scale(nw, nh)
            scaled.append(f"{w}x{h}->{nw}x{nh}")
        except Exception as exc:
            print(f"export_glb: could not scale {img.name!r} ({exc!r})", file=sys.stderr)
    return scaled


ensure_gltf_addon()

out_glb = params["output_path"]
print("export_glb: writing", out_glb)

# Texture downscale — the biggest lever on GLB size / Rhino import time.
texture_max_px = int(params.get("texture_max_px", 0) or 0)
scaled = downscale_images(texture_max_px)
if scaled:
    print(f"export_glb: downscaled {len(scaled)} image(s) to <={texture_max_px}px: " + ", ".join(scaled))
elif texture_max_px > 0:
    print(f"export_glb: no images exceeded {texture_max_px}px cap")

# Texture re-encode format. "AUTO" keeps the source format (glTF exporter
# default). WEBP is the best choice for the Rhino host: smallest, keeps
# alpha, and Rhino 8's glTF importer decodes it (verified — it unpacks WEBP
# to PNG on import).
export_kwargs = {
    "filepath": out_glb,
    "export_format": "GLB",
    "export_draco_mesh_compression_enable": bool(params.get("draco", False)),
    "export_yup": bool(params.get("yup", True)),
    "use_visible": False,
    "use_active_collection": False,
    "export_apply": bool(params.get("export_apply", True)),
}
image_format = str(params.get("image_format", "AUTO") or "AUTO").upper()
image_quality = int(params.get("image_quality", 85) or 85)
if image_format in ("WEBP", "JPEG", "PNG"):
    export_kwargs["export_image_format"] = image_format
    # export_image_quality only bites for the lossy formats; harmless for PNG.
    export_kwargs["export_image_quality"] = image_quality
    print(f"export_glb: image_format={image_format} quality={image_quality}")

try:
    bpy.ops.export_scene.gltf(**export_kwargs)
except TypeError as exc:
    # Older glTF exporters (pre-4.0) do not know export_image_format /
    # export_image_quality — retry without them rather than fail the convert.
    # Downscaling already did most of the work.
    if "export_image_format" in export_kwargs or "export_image_quality" in export_kwargs:
        print(
            f"export_glb: exporter rejected image kwargs ({exc!r}); retrying without them",
            file=sys.stderr,
        )
        export_kwargs.pop("export_image_format", None)
        export_kwargs.pop("export_image_quality", None)
        bpy.ops.export_scene.gltf(**export_kwargs)
    else:
        raise

print("export_glb: done")
