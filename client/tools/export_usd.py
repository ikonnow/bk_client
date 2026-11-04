"""Tools/export_usd.py - bundled Blendkit-client recipe.

Open a downloaded Blendkit .blend, unpack its packed textures into a
resolution-specific subfolder next to the .blend (mirroring the Blendkit
Blender addon's behaviour from ``unpack_asset_bg.py``), flatten any shader
node groups so the USD exporter can see Image Texture nodes, and export
the whole scene to a self-contained UsdPreviewSurface .usd file.

Recipe ABI:
    sys.argv = [..., "--", <params.json>]
    params.json keys:
        blend_path     : str  (required) - input .blend
        out_usd        : str  (required) - destination .usd
        max_resolution : str  (optional) - "512"/"1024"/"2048"/"4096"/"8192"/"ORIGINAL"
                                          used to pick the textures subfolder
                                          suffix (mirrors Blendkit addon)
        export_mtlx    : bool (optional, default True) - also emit standalone
                                          .mtlx materials into ``materials/`` and
                                          reference them from the .usd
                                          (via the vendored bk_mtlx exporter;
                                          skipped if MaterialX is unavailable)
        normal_map_target : str (optional, default "preserve") - normal-map
                                          convention for the exported MaterialX
                                          graphs ("preserve", "directx", or
                                          "unreal" for explicit signed decoding)

Stdout protocol (consumed by bk_maya.core.blender_runner)::
    BK_STATUS   <stage>
    BK_PROGRESS <0..1> <msg>
    BK_ARTIFACT <path>   (side outputs, e.g. the geometry crate)
    BK_DONE     <path>
    BK_ERROR    <msg>
"""

from __future__ import annotations

import contextlib
import json
import os
import re
import sys
import traceback
import xml.etree.ElementTree as ET
from typing import Any

import bpy  # type: ignore[import-not-found]

# ---------------------------------------------------------------------------
# Stdout protocol helpers
# ---------------------------------------------------------------------------


def _emit(tag: str, *parts: object) -> None:
    sys.stdout.write(f"{tag} {' '.join(str(p) for p in parts)}\n")
    sys.stdout.flush()


def status(s: str) -> None:
    """Emit a status update with the given stage name.

    Args:
        s: A string representing the current stage of the export process.
    """
    _emit("BK_STATUS", s)


def progress(frac: float, msg: str = "") -> None:
    """Emit a progress update with the given fraction (0..1) and optional message.

    Args:
        frac: A float between 0 and 1 indicating the completion percentage.
        msg: An optional string message providing additional context about the progress.
    """
    _emit("BK_PROGRESS", f"{frac:.3f}", msg)


def done(path: str) -> None:
    """Emit a done update with the given path.

    Args:
        path: A string representing the path of the completed export.
    """
    _emit("BK_DONE", path)


def artifact(path: str) -> None:
    """Emit a side-artifact path (e.g. the geometry crate).

    Distinct from BK_DONE (which marks the single primary output) so the
    consumer can pick up extra files without mistaking them for completion.

    Args:
        path: A string path to a produced side artifact.
    """
    _emit("BK_ARTIFACT", path)


def error(msg: str) -> None:
    """Emit an error update with the given message.

    Args:
        msg: A string representing the error message.
    """
    _emit("BK_ERROR", msg)


def log(msg: str) -> None:
    """Helper to log a message with the BK_PROGRESS tag (for diagnostic purposes).

    Args:
        msg: A string representing the log message.
    """
    print(f"[export_usd] {msg}", flush=True)


# ---------------------------------------------------------------------------
# Resolution → textures subfolder suffix (mirrors Blendkit addon paths.py)
# ---------------------------------------------------------------------------

# Same detection tokens as blendkit_addon/unpack_asset_bg.py::get_resolution_from_file_path
_RES_FROM_PATH_TOKENS = {
    "_0_5K_": "resolution_0_5K",
    "_1K_": "resolution_1K",
    "_2K_": "resolution_2K",
    "_4K_": "resolution_4K",
    "_8K_": "resolution_8K",
}


def _texture_subdir(blend_path: str) -> tuple[str, str]:
    """Return ``(rel_dir, abs_dir)`` for the textures subfolder next to *blend_path*.

    Always ``//textures/`` (no resolution suffix). The Blendkit-Maya cache
    is one folder per (asset, resolution), so resolution disambiguation is
    already handled at the directory level. Crucially, Blender's
    ``wm.usd_export`` MaterialX path always authors texture asset paths as
    ``./textures/<basename>`` regardless of ``image.filepath``, so the
    unpacked files MUST land in a folder literally named ``textures``.
    """
    rel_dir = "//textures/"
    abs_dir = bpy.path.abspath(rel_dir, start=os.path.dirname(blend_path) + os.sep)
    return rel_dir, abs_dir


# ---------------------------------------------------------------------------
# Texture unpacking (mirrors blendkit_addon/unpack_asset_bg.py::unpack_asset)
# ---------------------------------------------------------------------------


def _resolve_target_path(tex_rel_dir: str, image: bpy.types.Image, source_path: str = "") -> str:
    """Return a ``//``-relative target path for *image* inside *tex_rel_dir*.

    Mirrors ``unpack_asset_bg.py::get_texture_filepath`` — collision-resolves
    by appending ``000``, ``001``, ... when another image already claims the
    same filepath. Returned path uses forward slashes (Blender convention)
    and starts with ``//`` so it stays relative to the .blend.

    Args:
        tex_rel_dir: The relative directory (e.g., "//textures/") where the image should be placed.
        image: The Blender image object for which to resolve the target path.
        source_path: An optional string representing the original source path of the image,
            used for UDIM/sequence support.

    Returns:
        A string representing the resolved target path for the image, relative to the .blend file.
    """
    if source_path:
        path = source_path
    elif len(image.packed_files) > 0:
        path = image.packed_files[0].filepath
    else:
        path = image.filepath
    path = (path or "").replace("\\", "/")
    base_name = bpy.path.basename(path) or image.name.split(".")[0]

    # Always forward-slash, always blend-relative.
    original = tex_rel_dir.rstrip("/") + "/" + base_name
    final = original
    i = 0
    while True:
        clash = any(other is not image and other.filepath == final for other in bpy.data.images)
        if not clash:
            return final
        stem, ext = os.path.splitext(original)
        final = f"{stem}{str(i).zfill(3)}{ext}"
        i += 1


def unpack_textures(blend_path: str, *, progress_range: tuple[float, float] | None = None) -> str:
    """Unpack packed images and relocate them into ``//textures<suffix>/``.

    Args:
        blend_path: the source .blend, used to place the textures folder.
        progress_range: optional (start, end) fractions to report per image.

    Returns:
        The absolute textures directory path (for diagnostic only —
        Blender-side image paths stay ``//``-relative).
    """
    rel_dir, abs_dir = _texture_subdir(blend_path)
    os.makedirs(abs_dir, exist_ok=True)
    log(f"unpack: rel_dir={rel_dir} abs_dir={abs_dir}")

    bpy.data.use_autopack = False

    unpacked = 0
    relocated = 0
    images = [image for image in bpy.data.images if image.name != "Render Result"]
    for index, image in enumerate(images):
        if progress_range is not None:
            start, end = progress_range
            progress(start + (end - start) * index / len(images), f"Unpacking textures {index + 1}/{len(images)}")

        if len(image.packed_files) > 0:
            # UDIM / sequence support: per-packed-file target paths.
            paths = []
            for pf in image.packed_files:
                pf_path = _resolve_target_path(rel_dir, image, source_path=pf.filepath)
                pf.filepath = pf_path
                paths.append(pf_path)

            image_path = _resolve_target_path(rel_dir, image, source_path=image.filepath)
            image.filepath = image_path
            image.filepath_raw = image_path
            try:
                image.unpack(method="WRITE_ORIGINAL")
                unpacked += 1
                log(f"unpack: {image.name} -> {image_path} ({len(paths)} packed file(s))")
            except Exception as exc:
                log(f"unpack: FAILED for {image.name}: {exc}")
        else:
            fp = _resolve_target_path(rel_dir, image, source_path=image.filepath)
            if fp != image.filepath:
                image.filepath = fp
                image.filepath_raw = fp
                relocated += 1

    log(f"unpack: unpacked={unpacked} relocated={relocated}")
    return abs_dir


# ---------------------------------------------------------------------------
# Node group flattening
# ---------------------------------------------------------------------------
# Blender's wm.usd_export does NOT traverse inside material node groups.
# Many Blendkit assets wrap their shader in a "Blendkit Mat" group, so
# any Image Texture inside would be invisible to the exporter. Flatten every
# ShaderNodeGroup in every material before exporting.


def _flatten_material_node_groups() -> None:
    total = 0
    for mat in bpy.data.materials:
        if not mat.use_nodes or not mat.node_tree:
            continue
        n = _ungroup_all(mat.node_tree, mat)
        if n:
            log(f"flatten: {n} group(s) in material {mat.name!r}")
            total += n
    log(f"flatten: total groups expanded = {total}")


def _rename_shader_nodes_to_material() -> None:
    """Rename key shader nodes to carry the material name.

    Blender's USD exporter names emitted ``UsdPreviewSurface`` / ``UsdShade``
    prims after the *node name* — not the material datablock name. Default
    names like "Principled BSDF" / "Material Output" collide across materials
    in Maya/USD and make assets unreadable in the outliner. We rename:

    - the active output node           -> ``<Material>_Output``
    - the surface shader feeding it    -> ``<Material>_Surface``
    - every ShaderNodeBsdf*/Emission   -> ``<Material>_<orig-name-cleaned>``
    - every ShaderNodeTexImage         -> ``<Material>_<image-or-label>``
    """
    shader_ids = {
        "ShaderNodeBsdfPrincipled",
        "ShaderNodeBsdfDiffuse",
        "ShaderNodeBsdfGlossy",
        "ShaderNodeBsdfTransparent",
        "ShaderNodeBsdfGlass",
        "ShaderNodeBsdfRefraction",
        "ShaderNodeBsdfAnisotropic",
        "ShaderNodeBsdfHair",
        "ShaderNodeBsdfHairPrincipled",
        "ShaderNodeBsdfToon",
        "ShaderNodeBsdfVelvet",
        "ShaderNodeBsdfSheen",
        "ShaderNodeEmission",
        "ShaderNodeSubsurfaceScattering",
        "ShaderNodeMixShader",
        "ShaderNodeAddShader",
    }

    def _safe(name: str) -> str:
        # USD prim names: ascii, no spaces / dots / dashes.
        out = []
        for ch in name:
            if ch.isalnum() or ch == "_":
                out.append(ch)
            else:
                out.append("_")
        s = "".join(out).strip("_")
        return s or "Unnamed"

    total = 0
    for mat in bpy.data.materials:
        if not mat.use_nodes or not mat.node_tree:
            continue
        base = _safe(mat.name)
        nt = mat.node_tree

        # Find the active output node (the one Blender will export).
        output = None
        for n in nt.nodes:
            if n.bl_idname == "ShaderNodeOutputMaterial" and getattr(n, "is_active_output", False):
                output = n
                break
        if output is None:
            for n in nt.nodes:
                if n.bl_idname == "ShaderNodeOutputMaterial":
                    output = n
                    break

        if output is not None:
            output.name = f"{base}_Output"
            output.label = output.label or output.name
            total += 1
            # The node connected to its Surface input is the "main" shader.
            surf_in = output.inputs.get("Surface")
            if surf_in and surf_in.is_linked:
                src = surf_in.links[0].from_node
                src.name = f"{base}_Surface"
                src.label = src.label or src.name
                total += 1

        for n in nt.nodes:
            if n.bl_idname in shader_ids and not n.name.startswith(base + "_"):
                n.name = f"{base}_{_safe(n.bl_idname.replace('ShaderNode', ''))}"
                total += 1
            elif n.bl_idname == "ShaderNodeTexImage":
                img_name = ""
                if getattr(n, "image", None) is not None:
                    img_name = n.image.name
                tag = _safe(img_name or n.label or "Tex")
                if not n.name.startswith(base + "_"):
                    n.name = f"{base}_{tag}"
                    total += 1

    log(f"rename: shader nodes renamed = {total}")


def _ungroup_all(node_tree: bpy.types.NodeTree, owner: bpy.types.ID) -> int:
    """Recursively ungroup all ShaderNodeGroups in *node_tree*.

    Args:
        node_tree: A Blender node tree to process.
        owner: The owner of the node tree (e.g., a Material) for context during ungrouping.

    Returns:
        The total number of groups expanded.
    """
    expanded = 0
    safety = 32  # avoid infinite loops on pathological graphs
    while safety > 0:
        groups = [n for n in node_tree.nodes if n.bl_idname == "ShaderNodeGroup" and n.node_tree]
        if not groups:
            break
        safety -= 1
        for gn in groups:
            try:
                with bpy.context.temp_override(
                    window=bpy.context.window,
                    area=None,
                    region=None,
                    material=owner if isinstance(owner, bpy.types.Material) else None,
                    edit_tree=node_tree,
                    active_node=gn,
                    selected_nodes=[gn],
                ):
                    for n in node_tree.nodes:
                        n.select = False
                    gn.select = True
                    node_tree.nodes.active = gn
                    bpy.ops.node.group_ungroup()
                expanded += 1
            except Exception as exc:
                log(f"flatten: ungroup op failed for {gn.name!r}: {exc}; manual inline")
                if _manual_inline(node_tree, gn):
                    expanded += 1
    return expanded


def _manual_inline(parent_tree: bpy.types.NodeTree, group_node: bpy.types.Node) -> bool:
    """Fallback manual inlining if the group_ungroup operator fails (e.g. on complex node groups with reroutes).

    Args:
        parent_tree: The node tree into which the group should be inlined.
        group_node: The ShaderNodeGroup to be inlined.

    Returns:
        True if the group was successfully inlined, False otherwise.
    """
    try:
        inner = group_node.node_tree
        if inner is None:
            return False
        copies = {}
        for n in inner.nodes:
            if n.bl_idname in ("NodeGroupInput", "NodeGroupOutput"):
                continue
            new = parent_tree.nodes.new(n.bl_idname)
            for attr in ("label", "location", "width", "height"):
                with contextlib.suppress(Exception):
                    setattr(new, attr, getattr(n, attr))
            for prop in n.bl_rna.properties:
                if prop.is_readonly or prop.identifier in ("rna_type", "name"):
                    continue
                with contextlib.suppress(Exception):
                    setattr(new, prop.identifier, getattr(n, prop.identifier))
            copies[n] = new

        for lnk in inner.links:
            fn, fs = lnk.from_node, lnk.from_socket
            tn, ts = lnk.to_node, lnk.to_socket
            if fn.bl_idname == "NodeGroupInput":
                ext = group_node.inputs.get(fs.name)
                if ext and ext.is_linked:
                    parent_tree.links.new(ext.links[0].from_socket, copies[tn].inputs[ts.name])
                elif ext is not None:
                    with contextlib.suppress(Exception):
                        copies[tn].inputs[ts.name].default_value = ext.default_value

            elif tn.bl_idname == "NodeGroupOutput":
                ext = group_node.outputs.get(ts.name)
                if ext:
                    for ext_link in list(ext.links):
                        parent_tree.links.new(copies[fn].outputs[fs.name], ext_link.to_socket)
            else:
                parent_tree.links.new(copies[fn].outputs[fs.name], copies[tn].inputs[ts.name])

        parent_tree.nodes.remove(group_node)
        return True
    except Exception as exc:
        log(f"flatten: manual inline failed: {exc}")
        return False


# ---------------------------------------------------------------------------
# Single-material-per-mesh enforcement
# ---------------------------------------------------------------------------


def _is_rigged_or_unsplittable(obj: bpy.types.Object) -> str:
    """Return a non-empty reason string if *obj* must NOT be split, else ''.

    Splitting a mesh that participates in a rig destroys deformation
    fidelity (shape keys are outright refused by ``mesh.separate``;
    multires data is lost; cross-piece smooth deformation breaks at the
    new seams). For those meshes we leave the multi-material binding
    intact and rely on USD ``GeomSubset`` instead.

    Args:
        obj: A Blender object to check for rigging or unsplittable features.

    Returns:
        A non-empty string describing the reason why the object cannot be split, or an empty string
    """
    me = obj.data
    if me is None:
        return "no mesh data"
    if getattr(me, "shape_keys", None) is not None:
        return "has shape keys"
    for mod in obj.modifiers:
        mtype = getattr(mod, "type", "")
        if mtype in {"ARMATURE", "MULTIRES", "CLOTH", "SOFT_BODY", "MESH_DEFORM", "SURFACE_DEFORM"}:
            return f"has {mtype} modifier"
    if obj.parent is not None and obj.parent.type == "ARMATURE":
        return "parented to armature"
    if obj.parent_type in {"BONE", "VERTEX", "VERTEX_3"}:
        return f"parent_type={obj.parent_type}"
    return ""


def _split_meshes_by_material() -> None:
    """Ensure every (safely-splittable) mesh has exactly one material slot.

    Blender's ``wm.usd_export`` writes per-face material binding via
    ``GeomSubset``, but Maya's mayaUsd sometimes drops slots 1..N and shows
    everything with slot 0's material. Splitting works around that — but
    must NOT be done to rigged/deformed meshes (see _is_rigged_or_unsplittable).

    Purely in-memory: the source .blend was already saved earlier so the
    cache stays intact.
    """
    mesh_objs: list = []
    skipped: list[tuple[str, str]] = []
    for obj in list(bpy.context.scene.objects):
        if obj.type != "MESH" or obj.data is None:
            continue
        if len(obj.material_slots) <= 1:
            continue
        reason = _is_rigged_or_unsplittable(obj)
        if reason:
            skipped.append((obj.name, reason))
            continue
        mesh_objs.append(obj)

    for name, reason in skipped:
        log(f"split: SKIP {name!r} — {reason} (relying on USD GeomSubset)")

    if not mesh_objs:
        log("split: no splittable multi-material meshes")
        return

    with contextlib.suppress(Exception):
        bpy.ops.object.mode_set(mode="OBJECT")

    split_count = 0
    for obj in mesh_objs:
        n_slots = len(obj.material_slots)
        try:
            for o in bpy.context.scene.objects:
                with contextlib.suppress(Exception):
                    o.select_set(False)
            obj.select_set(True)
            bpy.context.view_layer.objects.active = obj

            with bpy.context.temp_override(
                window=bpy.context.window,
                scene=bpy.context.scene,
                view_layer=bpy.context.view_layer,
                active_object=obj,
                selected_objects=[obj],
                selected_editable_objects=[obj],
                object=obj,
                edit_object=obj,
            ):
                bpy.ops.object.mode_set(mode="EDIT")
                bpy.ops.mesh.select_all(action="SELECT")
                bpy.ops.mesh.separate(type="MATERIAL")
                bpy.ops.object.mode_set(mode="OBJECT")
            split_count += 1
            log(f"split: {obj.name!r} ({n_slots} slots)")
        except Exception as exc:
            log(f"split: FAILED for {obj.name!r}: {exc}")
            with contextlib.suppress(Exception):
                bpy.ops.object.mode_set(mode="OBJECT")

    # Collapse each resulting mesh down to its single actually-used material.
    # IMPORTANT: pick by the face's material_index (Blender's canonical
    # face→slot mapping). NEVER assume slot 0 — the assigned material is
    # frequently slot 3/5/7 etc.
    for obj in bpy.context.scene.objects:
        if obj.type != "MESH" or obj.data is None:
            continue
        # Don't touch meshes we deliberately left alone above.
        if _is_rigged_or_unsplittable(obj):
            continue
        try:
            polys = obj.data.polygons
            if len(polys) == 0:
                continue
            used_idx = {p.material_index for p in polys}
            # Resolve to actual material objects (multiple indices can map
            # to the same material if the source had duplicate slots).
            used_mats = []
            seen = set()
            for idx in used_idx:
                if 0 <= idx < len(obj.material_slots):  # noqa: SIM108
                    m = obj.material_slots[idx].material
                else:
                    m = None
                key = m.name if m else None
                if key in seen:
                    continue
                seen.add(key)
                used_mats.append((idx, m))

            if len(used_mats) == 1:
                kept_idx, kept_mat = used_mats[0]
                kept_name = kept_mat.name if kept_mat else "<None>"
                obj.data.materials.clear()
                if kept_mat is not None:
                    obj.data.materials.append(kept_mat)
                # Reset every polygon to slot 0 since we just cleared.
                for p in polys:
                    p.material_index = 0
                log(f"split: collapsed {obj.name!r} -> slot[{kept_idx}] {kept_name!r}")
            else:
                mat_list = [f"slot[{i}]={m.name if m else '<None>'}" for i, m in used_mats]
                log(
                    f"split: {obj.name!r} still uses {len(used_mats)} materials "
                    f"after separate — leaving slots intact ({', '.join(mat_list)})",
                )
        except Exception as exc:
            log(f"split: slot cleanup failed for {obj.name!r}: {exc}")

    log(f"split: processed {split_count} multi-material mesh(es), skipped {len(skipped)}")


def _force_single_material_per_mesh() -> None:
    """Reduce every mesh to EXACTLY one material slot.

    Runs after the split pass to catch:
    - meshes whose ``mesh.separate`` left multiple slots referenced
      (Blender refuses to split when only one slot is actually used
      and we don't catch that earlier);
    - rigged meshes we deliberately did NOT split (those still may have
      unused slots that Maya/USD mis-bind to).

    Strategy:
      - count faces per material_index;
      - pick the slot with the most faces (dominant material) — this is
        a destructive choice for genuinely multi-material rigged meshes,
        but per user request: "keep only one material per mesh, remove
        the assignments of others" because Maya GeomSubset binding is
        unreliable;
      - reassign all faces to slot 0, keep only that one material.
    """
    from collections import Counter

    forced = 0
    for obj in bpy.context.scene.objects:
        if obj.type != "MESH" or obj.data is None:
            continue
        slots = obj.material_slots
        if len(slots) <= 1:
            continue
        polys = obj.data.polygons
        if len(polys) == 0:
            continue

        hist = Counter(p.material_index for p in polys)
        # Dominant slot index (the one most faces use).
        dom_idx, dom_count = hist.most_common(1)[0]
        dom_mat = slots[dom_idx].material if 0 <= dom_idx < len(slots) else None
        dom_name = dom_mat.name if dom_mat else "<None>"
        total = len(polys)
        lost = total - dom_count

        # Clear all slots then re-append only the dominant material.
        obj.data.materials.clear()
        if dom_mat is not None:
            obj.data.materials.append(dom_mat)
        for p in polys:
            p.material_index = 0
        forced += 1

        msg = f"force-single: {obj.name!r} -> {dom_name!r} (was slot[{dom_idx}], {dom_count}/{total} faces"
        if lost > 0:
            msg += f", {lost} face(s) reassigned from other slots"
        msg += ")"
        log(msg)

    log(f"force-single: reduced {forced} mesh(es) to single material")


def _remove_empty_uv_layers() -> None:
    """Drop UV layers that contain no usable data.

    A layer is considered empty if every UV is at (0, 0) or every face
    has zero UV area (a bake/scratch layer that was never unwrapped).
    Never removes the last remaining layer, and never removes a layer
    referenced by a ``ShaderNodeUVMap`` in any of the mesh's materials.
    """
    removed_total = 0
    for obj in bpy.context.scene.objects:
        if obj.type != "MESH" or obj.data is None:
            continue
        mesh = obj.data
        layers = mesh.uv_layers
        if len(layers) <= 1 or not mesh.polygons:
            continue

        # Names referenced by shader UV-Map nodes (never delete these).
        referenced: set[str] = set()
        for slot in obj.material_slots:
            mat = slot.material
            if mat is None or not mat.use_nodes or mat.node_tree is None:
                continue
            for node in mat.node_tree.nodes:
                if node.bl_idname == "ShaderNodeUVMap" and node.uv_map:
                    referenced.add(node.uv_map)

        to_remove = []
        for lyr in list(layers):
            if lyr.name in referenced:
                continue
            data = lyr.data
            # All-zero check: very cheap, catches the common case.
            all_zero = True
            for d in data:
                u, v = d.uv
                if u != 0.0 or v != 0.0:
                    all_zero = False
                    break
            if all_zero:
                to_remove.append(lyr.name)
                continue
            # Zero-area check: every face has degenerate UVs.
            any_area = False
            for poly in mesh.polygons:
                start = poly.loop_start
                n = poly.loop_total
                if n < 3:
                    continue
                # Shoelace area.
                area = 0.0
                u0, v0 = data[start].uv
                for i in range(1, n - 1):
                    u1, v1 = data[start + i].uv
                    u2, v2 = data[start + i + 1].uv
                    area += abs((u1 - u0) * (v2 - v0) - (u2 - u0) * (v1 - v0))
                if area > 1e-12:
                    any_area = True
                    break
            if not any_area:
                to_remove.append(lyr.name)

        # Keep at least one layer.
        if len(to_remove) >= len(layers):
            to_remove = to_remove[: len(layers) - 1]

        for name in to_remove:
            try:
                layers.remove(layers[name])
                removed_total += 1
            except Exception as exc:
                log(f"remove-empty-uv: failed on {obj.name!r}/{name!r}: {exc}")

        if to_remove:
            log(f"remove-empty-uv: {obj.name!r} dropped {to_remove}")

    log(f"remove-empty-uv: removed {removed_total} empty layer(s)")


def _fix_primary_uv_set() -> None:
    """Ensure the *correct* UV layer is marked ``active_render`` per mesh.

    Blender's ``wm.usd_export`` exports every UV layer, but the one
    flagged ``active_render`` becomes the primary ``st`` set in USD —
    i.e. the one Maya's shaders bind to by default. If an asset's
    ``active_render`` happens to be a bake/scratch UV (``Bake_Iris``,
    ``Gradient``, ...), the imported mesh in Maya will use that wrong
    set and the textures will look mangled.

    Heuristic, in order:
      1. The UV layer name referenced by a ``ShaderNodeUVMap`` that
         feeds an Image Texture in any material on the mesh.
      2. Otherwise the layer currently marked ``active`` (what the
         artist sees in the UV editor).
      3. Otherwise ``uv_layers[0]``.
    """
    from collections import Counter

    fixed = 0
    for obj in bpy.context.scene.objects:
        if obj.type != "MESH" or obj.data is None:
            continue
        mesh = obj.data
        layers = mesh.uv_layers
        if len(layers) == 0:
            continue

        # 1) gather UV-Map node names driving image textures
        used_names: Counter = Counter()
        for slot in obj.material_slots:
            mat = slot.material
            if mat is None or not mat.use_nodes or mat.node_tree is None:
                continue
            nt = mat.node_tree
            # Find image-texture nodes; walk back along their Vector input
            # to spot the UVMap node feeding them.
            for node in nt.nodes:
                if node.bl_idname != "ShaderNodeTexImage":
                    continue
                vec_in = node.inputs.get("Vector")
                if vec_in is None or not vec_in.is_linked:
                    continue
                src = vec_in.links[0].from_node
                # Walk through Mapping nodes if present
                while src.bl_idname == "ShaderNodeMapping":
                    m_in = src.inputs.get("Vector")
                    if m_in is None or not m_in.is_linked:
                        src = None  # type: ignore[assignment]
                        break
                    src = m_in.links[0].from_node
                if src is None:
                    continue
                if src.bl_idname == "ShaderNodeUVMap" and src.uv_map:
                    used_names[src.uv_map] += 1

        chosen = ""
        for name, _ in used_names.most_common():
            if name in layers:
                chosen = name
                break

        # 2) fallback: currently active UV layer
        if not chosen and layers.active is not None:
            chosen = layers.active.name

        # 3) fallback: first layer
        if not chosen:
            chosen = layers[0].name

        # Apply: mark chosen as both active and active_render.
        for lyr in layers:
            lyr.active_render = lyr.name == chosen
        with contextlib.suppress(Exception):
            layers.active = layers[chosen]

        fixed += 1
        if used_names:
            log(f"primary-uv: {obj.name!r} -> {chosen!r} (from material nodes; candidates={dict(used_names)})")
        else:
            log(f"primary-uv: {obj.name!r} -> {chosen!r} (fallback)")

    log(f"primary-uv: fixed {fixed} mesh(es)")


def _sanitize_meshes_for_usd_export() -> None:
    """Rebuild mesh CustomData so the USD exporter writes valid UV indices.

    Blender's ``wm.usd_export`` deduplicates faceVarying UVs into a
    ``values`` + ``indices`` pair. After destructive mesh ops (separate
    by material, slot pruning) the CustomData layout can get into a
    state where the exporter writes the compressed ``values`` array but
    silently omits the ``indices`` array — making the UVs unusable in
    Maya (values count != loop count, no indices to gather from).

    Round-tripping through ``object.convert(target='MESH')`` rebuilds
    the mesh data block from the evaluated depsgraph, producing a clean
    CustomData layout that the exporter handles correctly.

    Also calls ``mesh.validate()`` + ``mesh.update()`` as belt-and-braces.
    """
    scene = bpy.context.scene
    view_layer = bpy.context.view_layer

    # Work on a stable snapshot of mesh objects.
    mesh_objs = [o for o in scene.objects if o.type == "MESH" and o.data]
    if not mesh_objs:
        return

    # Deselect all, then convert each mesh individually to keep memory
    # bounded and isolate failures.
    with contextlib.suppress(Exception):
        bpy.ops.object.select_all(action="DESELECT")

    rebuilt = 0
    failed = 0
    for obj in mesh_objs:
        try:
            # Skip linked / library data we can't convert.
            if obj.data.library is not None:
                continue
            # convert() writes the evaluated result back into the mesh datablock. Instances sharing
            # it would then re-apply their modifiers to already-modified geometry (a Bevel on 24
            # linked duplicates grew exponentially until Blender exhausted RAM), so unshare first.
            if obj.data.users > 1 and obj.modifiers:
                obj.data = obj.data.copy()
            # Select & activate just this object.
            with contextlib.suppress(Exception):
                bpy.ops.object.select_all(action="DESELECT")
            obj.select_set(True)
            view_layer.objects.active = obj
            # Re-bake the mesh from the evaluated depsgraph.
            bpy.ops.object.convert(target="MESH")
            mesh = obj.data
            mesh.validate(verbose=False)
            mesh.update()
            rebuilt += 1
        except Exception as exc:
            failed += 1
            log(f"sanitize: convert failed for {obj.name!r}: {exc}")

    with contextlib.suppress(Exception):
        bpy.ops.object.select_all(action="DESELECT")

    log(f"sanitize: rebuilt {rebuilt} mesh(es), {failed} failed")


# ---------------------------------------------------------------------------
# MaterialX sidecar export (vendored bl_mtlx exporter)
# ---------------------------------------------------------------------------
# Blender's native wm.usd_export MaterialX network only covers the subset of
# nodes it can map to a UsdPreviewSurface-equivalent graph. The vendored
# bl_mtlx exporter emits a far richer standalone .mtlx per material (image /
# math / mix / ramp / noise / voronoi / displacement / ...). We export those
# next to the .usd and reference each one onto its material prim so the
# consumer (Maya mayaUsd / MaterialX) binds the high-fidelity materials onto
# the same geometry.
#
# The compiled ``MaterialX`` (PyMaterialX) package is NOT bundled — it ships
# with Blender 4.1+. If it is missing we skip this step and still emit the USD.


def _safe_filename(name: str) -> str:
    """Return *name* reduced to a filesystem/USD-safe token (ascii, no spaces)."""
    out = [ch if (ch.isalnum() or ch in ("_", "-")) else "_" for ch in name]
    return "".join(out).strip("_") or "Unnamed"


def _load_mtlx_exporter():
    """Import the vendored bl_mtlx exporter under its unique package name.

    Returns the ``export_material_to_materialx`` callable, or ``None`` if the
    package (or its compiled ``MaterialX`` dependency) is unavailable. Loaded
    as ``bk_mtlx`` so it never clashes with the ``material_x`` add-on when that
    is also enabled in Blender.
    """
    script_dir = os.path.dirname(os.path.abspath(__file__))
    if script_dir not in sys.path:
        sys.path.insert(0, script_dir)
    try:
        from bk_mtlx.blender_materialx_exporter import export_material_to_materialx
    except ImportError as exc:
        log(f"mtlx: exporter/MaterialX unavailable ({exc}); skipping .mtlx export")
        return None
    return export_material_to_materialx


def _fix_mtlx_normal_maps(mtlx_path: str) -> int:
    """Repair mistranslated normal-map connections in a generated .mtlx.

    Insert a normalmap on incorrectly typed bump-normal connections without
    replacing the bump or dropping its height input. Normal textures are raw
    data. Returns the number of connections fixed.
    """
    try:
        # Parsing our own generated, trusted .mtlx (not untrusted input).
        tree = ET.parse(mtlx_path)  # noqa: S314  # nosec B314
    except (ET.ParseError, OSError) as exc:
        log(f"mtlx-fix: could not parse {os.path.basename(mtlx_path)} ({exc})")
        return 0

    root = tree.getroot()
    fixed = 0
    for graph in root.iter("nodegraph"):
        by_name = {n.get("name"): n for n in graph if n.get("name")}
        for node in list(graph):
            if node.tag != "bump":
                continue
            nrm = node.find("input[@name='normal']")
            if nrm is None:
                continue
            src = by_name.get(nrm.get("nodename"))
            if src is None or src.tag != "image" or src.get("type") == "vector3":
                continue
            src.set("type", "vector3")
            if src.get("nodedef"):
                src.set("nodedef", "ND_image_vector3")
            src.set("colorspace", "raw")
            normal_name = f"bk_normalmap_{node.get('name')}"
            while normal_name in by_name:
                normal_name += "_"
            normal = ET.SubElement(
                graph, "normalmap", {"name": normal_name, "type": "vector3", "nodedef": "ND_normalmap_float"}
            )
            ET.SubElement(normal, "input", {"name": "in", "type": "vector3", "nodename": src.get("name")})
            by_name[normal_name] = normal
            nrm.set("nodename", normal_name)
            fixed += 1

    if fixed:
        # ET.indent is 3.9+, headless Blender is newer — but be safe
        with contextlib.suppress(AttributeError):
            ET.indent(tree, space="  ")
        tree.write(mtlx_path, encoding="unicode", xml_declaration=True)
        log(f"mtlx-fix: repaired {fixed} normal-map connection(s) in {os.path.basename(mtlx_path)}")
    return fixed


def _convert_mtlx_normal_maps_to_directx(mtlx_path: str) -> int:
    """Flip the green channel of each connected tangent-space normal map.

    Blender's tangent-space normal maps use the OpenGL convention. MaterialX
    has no portable per-map convention flag, so express the DirectX conversion
    in the graph as ``G = 1 - G`` without modifying shared texture files.
    """
    try:
        # Parsing our own generated, trusted .mtlx (not untrusted input).
        tree = ET.parse(mtlx_path)  # noqa: S314  # nosec B314
    except (ET.ParseError, OSError) as exc:
        log(f"mtlx-normal: could not parse {os.path.basename(mtlx_path)} ({exc})")
        return 0

    root = tree.getroot()
    converted = 0
    for graph in root.iter("nodegraph"):
        existing_names = {node.get("name") for node in graph if node.get("name")}
        for node in list(graph):
            if node.tag != "normalmap":
                continue
            normal_input = node.find("input[@name='in']")
            if normal_input is None:
                continue
            source_name = normal_input.get("nodename")
            if not source_name:
                continue

            source_node = next(
                (child for child in graph if child.get("name") == source_name),
                None,
            )
            if (
                source_node is not None
                and source_node.tag == "combine3"
                and source_name.startswith("bk_dx_normal_combine_")
            ):
                continue

            index = converted
            while True:
                separate_name = f"bk_dx_normal_separate_{index}"
                subtract_name = f"bk_dx_normal_subtract_{index}"
                combine_name = f"bk_dx_normal_combine_{index}"
                if not ({separate_name, subtract_name, combine_name} & existing_names):
                    break
                index += 1
            existing_names.update((separate_name, subtract_name, combine_name))

            separate = ET.Element(
                "separate3",
                {"name": separate_name, "type": "multioutput", "nodedef": "ND_separate3_vector3"},
            )
            separate_input = ET.SubElement(
                separate,
                "input",
                {"name": "in", "type": "vector3", "nodename": source_name},
            )
            source_output = normal_input.get("output")
            if source_output:
                separate_input.set("output", source_output)

            subtract = ET.Element(
                "subtract",
                {"name": subtract_name, "type": "float", "nodedef": "ND_subtract_float"},
            )
            ET.SubElement(subtract, "input", {"name": "in1", "type": "float", "value": "1"})
            ET.SubElement(
                subtract,
                "input",
                {"name": "in2", "type": "float", "nodename": separate_name, "output": "outy"},
            )

            combine = ET.Element(
                "combine3",
                {"name": combine_name, "type": "vector3", "nodedef": "ND_combine3_vector3"},
            )
            ET.SubElement(
                combine,
                "input",
                {"name": "in1", "type": "float", "nodename": separate_name, "output": "outx"},
            )
            ET.SubElement(
                combine,
                "input",
                {"name": "in2", "type": "float", "nodename": subtract_name, "output": "out"},
            )
            ET.SubElement(
                combine,
                "input",
                {"name": "in3", "type": "float", "nodename": separate_name, "output": "outz"},
            )

            node_index = list(graph).index(node)
            graph.insert(node_index, separate)
            graph.insert(node_index + 1, subtract)
            graph.insert(node_index + 2, combine)
            normal_input.set("nodename", combine_name)
            normal_input.attrib.pop("output", None)
            converted += 1

    if converted:
        with contextlib.suppress(AttributeError):
            ET.indent(tree, space="  ")
        tree.write(mtlx_path, encoding="unicode", xml_declaration=True)
        log(f"mtlx-normal: converted {converted} tangent-space normal map(s) to DirectX")
    return converted


def _prepare_mtlx_normals_for_unreal(mtlx_path: str, *, material_name: str = "") -> int:
    """Decode encoded normal inputs once into Unreal's signed tangent space.

    Unreal's MaterialX translator treats normalmap as a pass-through and
    automatically unpacks normal image samples. Re-encode those samples before
    upstream image arithmetic, then decode with (2, -2, 2) and (-1, 1, -1).
    This mode targets Unreal, not a conforming MaterialX renderer.
    """
    tree = ET.parse(mtlx_path)  # noqa: S314  # nosec B314
    converted = 0
    for graph in tree.getroot().iter("nodegraph"):
        by_name = {node.get("name"): node for node in graph if node.get("name")}

        def unique_name(base: str, names: dict = by_name) -> str:
            name = base
            while name in names:
                name += "_"
            names[name] = None
            return name

        def encoded_images(
            source_name: str, visited: set[str], nodes: dict = by_name, parent: ET.Element = graph
        ) -> None:
            if source_name in visited:
                return
            visited.add(source_name)
            source = nodes.get(source_name)
            if source is None:
                return
            if source.get("blendkit_normal_encoding") == "encoded":
                return
            if source.tag == "image":
                source_type = source.get("type")
                if source_type not in ("color3", "vector3"):
                    raise ValueError(f"Unsupported normal image type: {source_type}")
                source.set("colorspace", "raw")
                file_input = source.find("input[@name='file']")
                if file_input is not None:
                    file_input.set("colorspace", "raw")
                image_name = unique_name(f"{source_name}_signed")
                source.set("name", image_name)
                source.set("type", "vector3")
                source.set("nodedef", "ND_image_vector3")
                scale_name = unique_name(f"{source_name}_encode_scale")
                bias_name = unique_name(f"{source_name}_encode_bias")
                scale = ET.SubElement(
                    parent, "multiply", {"name": scale_name, "type": "vector3", "nodedef": "ND_multiply_vector3"}
                )
                ET.SubElement(scale, "input", {"name": "in1", "type": "vector3", "nodename": image_name})
                ET.SubElement(scale, "input", {"name": "in2", "type": "vector3", "value": "0.5,0.5,0.5"})
                bias = ET.SubElement(parent, "add", {"name": bias_name, "type": "vector3", "nodedef": "ND_add_vector3"})
                ET.SubElement(bias, "input", {"name": "in1", "type": "vector3", "nodename": scale_name})
                ET.SubElement(bias, "input", {"name": "in2", "type": "vector3", "value": "0.5,0.5,0.5"})
                convert = ET.SubElement(
                    parent,
                    "convert",
                    {
                        "name": source_name,
                        "type": source_type,
                        "nodedef": f"ND_convert_vector3_{source_type}",
                        "blendkit_normal_encoding": "encoded",
                    },
                )
                if source_type == "vector3":
                    convert.tag = "dot"
                    convert.set("nodedef", "ND_dot_vector3")
                ET.SubElement(convert, "input", {"name": "in", "type": "vector3", "nodename": bias_name})
                nodes[image_name] = source
                nodes[source_name] = convert
                return
            for input_node in source.findall("input"):
                upstream = input_node.get("nodename")
                if upstream:
                    encoded_images(upstream, visited)

        for node in list(graph):
            if node.tag != "normalmap" or node.get("blendkit_normal_decoding") == "signed_directx":
                continue
            normal_input = node.find("input[@name='in']")
            if normal_input is None:
                continue
            source_name = normal_input.get("nodename")
            if source_name:
                encoded_images(source_name, set())
            scale_name = unique_name(f"bk_unreal_normal_scale_{converted}")
            bias_name = unique_name(f"bk_unreal_normal_bias_{converted}")
            scale = ET.SubElement(
                graph, "multiply", {"name": scale_name, "type": "vector3", "nodedef": "ND_multiply_vector3"}
            )
            scale_input = ET.SubElement(scale, "input", dict(normal_input.attrib))
            scale_input.set("name", "in1")
            ET.SubElement(scale, "input", {"name": "in2", "type": "vector3", "value": "2,-2,2"})
            bias = ET.SubElement(graph, "add", {"name": bias_name, "type": "vector3", "nodedef": "ND_add_vector3"})
            ET.SubElement(bias, "input", {"name": "in1", "type": "vector3", "nodename": scale_name})
            ET.SubElement(bias, "input", {"name": "in2", "type": "vector3", "value": "-1,1,-1"})
            normal_input.attrib.clear()
            normal_input.attrib.update({"name": "in", "type": "vector3", "nodename": bias_name})
            node.set("blendkit_normal_decoding", "signed_directx")
            converted += 1
        for node in list(graph):
            if node.tag != "bump":
                continue
            height_name = unique_name(f"bk_unreal_height_{node.get('name')}")
            height = ET.SubElement(
                graph,
                "heighttonormal",
                {"name": height_name, "type": "vector3", "nodedef": "ND_heighttonormal_vector3"},
            )
            height_input = node.find("input[@name='height']")
            height_value = {"type": "float", "value": "0"} if height_input is None else dict(height_input.attrib)
            height_value["name"] = "in"
            ET.SubElement(height, "input", height_value)
            scale_input = node.find("input[@name='scale']")
            if scale_input is not None:
                ET.SubElement(height, "input", dict(scale_input.attrib))
            normal_input = node.find("input[@name='normal']")
            if normal_input is None:
                result_name = height_name
            else:
                normal_value = dict(normal_input.attrib)
                sum_name = unique_name(f"{height_name}_sum")
                product_name = unique_name(f"{height_name}_product")
                xy_name = unique_name(f"{height_name}_xy")
                z_name = unique_name(f"{height_name}_z")
                result_name = unique_name(f"{height_name}_combined")
                for tag, name in (("add", sum_name), ("multiply", product_name)):
                    operation = ET.SubElement(
                        graph, tag, {"name": name, "type": "vector3", "nodedef": f"ND_{tag}_vector3"}
                    )
                    ET.SubElement(operation, "input", {**normal_value, "name": "in1"})
                    ET.SubElement(operation, "input", {"name": "in2", "type": "vector3", "nodename": height_name})
                for name, source, mask in ((xy_name, sum_name, "1,1,0"), (z_name, product_name, "0,0,1")):
                    operation = ET.SubElement(
                        graph, "multiply", {"name": name, "type": "vector3", "nodedef": "ND_multiply_vector3"}
                    )
                    ET.SubElement(operation, "input", {"name": "in1", "type": "vector3", "nodename": source})
                    ET.SubElement(operation, "input", {"name": "in2", "type": "vector3", "value": mask})
                combined = ET.SubElement(
                    graph, "add", {"name": result_name, "type": "vector3", "nodedef": "ND_add_vector3"}
                )
                ET.SubElement(combined, "input", {"name": "in1", "type": "vector3", "nodename": xy_name})
                ET.SubElement(combined, "input", {"name": "in2", "type": "vector3", "nodename": z_name})
            # Unreal drops bump's base normal when expanding it; combine signed normals explicitly.
            node.tag = "normalize"
            node.set("nodedef", "ND_normalize_vector3")
            for input_node in list(node):
                node.remove(input_node)
            ET.SubElement(node, "input", {"name": "in", "type": "vector3", "nodename": result_name})
            converted += 1
    renamed = False
    root = tree.getroot()
    if material_name:
        material_name = _usd_safe_name(material_name)
        surfaces = root.findall("surfacematerial")
        if len(surfaces) != 1:
            raise ValueError(f"Expected one MaterialX material in {mtlx_path}")
        surface = surfaces[0]
        for child in root:
            if child is surface or child.get("name") != material_name:
                continue
            if child.tag != "nodegraph":
                raise ValueError(f"MaterialX name collision for {material_name}")
            names = {element.get("name") for element in root}
            graph_name = f"NG_{material_name}"
            while graph_name in names:
                graph_name += "_"
            child.set("name", graph_name)
            for element in root.iter():
                if element.get("nodegraph") == material_name:
                    element.set("nodegraph", graph_name)
            renamed = True
        if surface.get("name") != material_name:
            surface.set("name", material_name)
            renamed = True
    if converted or renamed:
        ET.indent(tree, space="  ")
        tree.write(mtlx_path, encoding="unicode", xml_declaration=True)
        log(f"mtlx-normal: explicitly decoded {converted} normal map(s) for Unreal")
    return converted


def _sanitize_mtlx_names(mtlx_path: str) -> int:
    """Make MaterialX element names valid USD identifiers and update references.

    usdMtlx maps nodegraphs and nodes to prims; a name such as ``06_Material``
    fails its prim-path verification and aborts stage composition.

    Args:
        mtlx_path: the .mtlx document to rewrite in place.

    Returns:
        The number of renamed elements.
    """
    tree = ET.parse(mtlx_path)  # noqa: S314  # nosec B314
    root = tree.getroot()

    def _rename_scope(parent: ET.Element) -> dict[str, str]:
        elements = [child for child in parent if child.get("name") and child.tag not in ("input", "output")]
        taken = {element.get("name") for element in elements}
        renames: dict[str, str] = {}
        for element in elements:
            name = element.get("name") or ""
            safe = _usd_safe_name(name)
            if safe == name:
                continue
            while safe in taken:
                safe += "_"
            taken.add(safe)
            renames[name] = safe
            element.set("name", safe)
        return renames

    root_renames = _rename_scope(root)
    renamed = len(root_renames)
    for element in root.iter():
        graph = element.get("nodegraph")
        if graph in root_renames:
            element.set("nodegraph", root_renames[graph])
    for child in root:
        for port in child:
            if port.tag == "input" and port.get("nodename") in root_renames:
                port.set("nodename", root_renames[port.get("nodename")])
    for graph in root.iter("nodegraph"):
        graph_renames = _rename_scope(graph)
        renamed += len(graph_renames)
        for element in graph.iter():
            if element.get("nodename") in graph_renames:
                element.set("nodename", graph_renames[element.get("nodename")])
    if renamed:
        ET.indent(tree, space="  ")
        tree.write(mtlx_path, encoding="unicode", xml_declaration=True)
        log(f"mtlx-names: renamed {renamed} element(s) to valid USD identifiers")
    return renamed


def _fix_mtlx_scalar_connections(mtlx_path: str) -> int:
    """Make implicit Blender color-to-scalar casts explicit for MaterialX."""
    tree = ET.parse(mtlx_path)  # noqa: S314  # nosec B314
    fixed = 0
    for graph in tree.getroot().iter("nodegraph"):
        by_name = {node.get("name"): node for node in graph if node.get("name")}
        # Nodegraph <output>s feed float shader inputs too (e.g. a color graph -> specular_roughness).
        ports = [port for node in list(graph) for port in node.findall("input")] + graph.findall("output")
        for input_node in ports:
            source = by_name.get(input_node.get("nodename"))
            if (
                input_node.get("type") != "float"
                or input_node.get("output")
                or source is None
                or source.get("type") not in ("color3", "vector3", "color4", "vector4")
            ):
                continue
            name = f"bk_scalar_{fixed}"
            while name in by_name or f"{name}_vector" in by_name:
                name += "_"
            source_type = source.get("type")
            vector_type = f"vector{source_type[-1]}"
            source_name = source.get("name")
            if source_type.startswith("color"):
                vector = ET.SubElement(
                    graph,
                    "convert",
                    {
                        "name": f"{name}_vector",
                        "type": vector_type,
                        "nodedef": f"ND_convert_{source_type}_{vector_type}",
                    },
                )
                ET.SubElement(vector, "input", {"name": "in", "type": source_type, "nodename": source_name})
                source_name = f"{name}_vector"
                by_name[source_name] = vector
            conversion = ET.SubElement(
                graph,
                "dotproduct",
                {"name": name, "type": "float", "nodedef": f"ND_dotproduct_{vector_type}"},
            )
            ET.SubElement(conversion, "input", {"name": "in1", "type": vector_type, "nodename": source_name})
            ET.SubElement(
                conversion,
                "input",
                {
                    "name": "in2",
                    "type": vector_type,
                    "value": "0.3333333333,0.3333333333,0.3333333333" + (",0" if vector_type == "vector4" else ""),
                },
            )
            by_name[name] = conversion
            input_node.set("nodename", name)
            fixed += 1
    if fixed:
        ET.indent(tree, space="  ")
        tree.write(mtlx_path, encoding="unicode", xml_declaration=True)
        log(f"mtlx-fix: made {fixed} scalar connection(s) explicit")
    return fixed


_MTLX_STDLIB: dict[str, Any] = {}


def _mtlx_stdlib() -> Any:
    """MaterialX standard data library (cached), or None outside a MaterialX-enabled Python."""
    if "library" not in _MTLX_STDLIB:
        try:
            import MaterialX  # bundled with Blender, absent from the test venv
        except ImportError:
            _MTLX_STDLIB["library"] = None
        else:
            library = MaterialX.createDocument()
            MaterialX.loadLibraries(
                MaterialX.getDefaultDataLibraryFolders(), MaterialX.getDefaultDataSearchPath(), library
            )
            _MTLX_STDLIB["library"] = library
    return _MTLX_STDLIB["library"]


def _mtlx_port_source_type(library: Any, source: ET.Element, output: str) -> str:
    if not output:
        return "" if source.get("type") == "multioutput" else source.get("type") or ""
    nodedef = library.getNodeDef(source.get("nodedef") or "")
    port = nodedef.getActiveOutput(output) if nodedef is not None else None
    return port.getType() if port is not None else ""


def _repair_mtlx_types(mtlx_path: str, library: Any = None) -> int:
    """Insert stdlib ``convert`` nodes wherever a connection's types don't match.

    Blender casts freely between colors/vectors/floats (color ramp RGBA into a
    color socket, an image into a vector math node); MaterialX rejects those
    connections and Unreal Interchange then drops the whole material.

    Args:
        mtlx_path: the .mtlx document to rewrite in place.
        library: MaterialX data library for nodedef lookups (defaults to the stdlib).

    Returns:
        The number of repaired connections.
    """
    library = library or _mtlx_stdlib()
    if library is None:
        return 0
    tree = ET.parse(mtlx_path)  # noqa: S314  # nosec B314
    fixed = 0
    for graph in tree.getroot().iter("nodegraph"):
        by_name = {node.get("name"): node for node in graph if node.get("name") and node.tag != "output"}
        ports = [port for node in list(graph) for port in node.findall("input")] + graph.findall("output")
        converts: dict[tuple[str, str, str], str] = {}
        for port in ports:
            source = by_name.get(port.get("nodename"))
            target_type = port.get("type") or ""
            output = port.get("output") or ""
            if source is None:
                continue
            source_type = _mtlx_port_source_type(library, source, output)
            nodedef = f"ND_convert_{source_type}_{target_type}"
            if not source_type or source_type == target_type or library.getNodeDef(nodedef) is None:
                continue
            key = (source.get("name") or "", output, target_type)
            name = converts.get(key)
            if name is None:
                name = f"bk_convert_{len(converts)}"
                while name in by_name:
                    name += "_"
                convert = ET.SubElement(graph, "convert", {"name": name, "type": target_type, "nodedef": nodedef})
                source_port = {"name": "in", "type": source_type, "nodename": key[0]}
                if output:
                    source_port["output"] = output
                ET.SubElement(convert, "input", source_port)
                by_name[name] = convert
                converts[key] = name
            port.set("nodename", name)
            port.attrib.pop("output", None)
            fixed += 1
    if fixed:
        ET.indent(tree, space="  ")
        tree.write(mtlx_path, encoding="unicode", xml_declaration=True)
        log(f"mtlx-fix: converted {fixed} mismatched connection(s)")
    return fixed


def _validate_mtlx(mtlx_path: str) -> str:
    """Run MaterialX's own document validation; returns "" when valid (or MaterialX is unavailable)."""
    library = _mtlx_stdlib()
    if library is None:
        return ""
    import MaterialX

    document = MaterialX.createDocument()
    MaterialX.readFromXmlFile(document, mtlx_path)
    document.importLibrary(library)
    valid, message = document.validate()
    return "" if valid else " | ".join(line.strip() for line in message.splitlines() if line.strip())


def _author_usd_normal_metadata(usd_path: str, normal_map_target: str) -> int:
    """Record normal conventions and native Preview Surface texture decoding.

    Custom data documents interpretation; shader inputs carry the actual math.
    A texture's original convention remains unknown when an upstream graph
    may already compensate it. Blender Normal Map inputs expect OpenGL.
    """
    from pxr import Gf, Sdf, Usd, UsdShade

    stage = Usd.Stage.Open(usd_path)
    if stage is None:
        raise RuntimeError(f"Could not open exported USD stage: {usd_path}")
    source_conventions = {}
    for image in bpy.data.images:
        convention = image.get("blendkit_normal_convention", "unknown")
        if convention not in ("opengl", "directx", "unknown"):
            raise ValueError(f"Invalid blendkit_normal_convention on image {image.name!r}: {convention!r}")
        source_conventions[os.path.basename(bpy.path.abspath(image.filepath))] = convention
    authored = 0
    for prim in stage.Traverse():
        shader = UsdShade.Shader(prim)
        if not shader:
            continue
        shader_id = str(shader.GetIdAttr().Get() or "")
        if shader_id == "ND_image_vector3":
            file_input = shader.GetInput("file")
            file_path = file_input.Get() if file_input else None
            if file_path:
                prim.SetCustomDataByKey(
                    "blendkit:normal:sourceConvention",
                    source_conventions.get(os.path.basename(file_path.path), "unknown"),
                )
        if shader_id.startswith("ND_normalmap"):
            prim.SetCustomDataByKey("blendkit:normal:inputConvention", "opengl")
            prim.SetCustomDataByKey("blendkit:normal:sourceConvention", "unknown")
            prim.SetCustomDataByKey(
                "blendkit:normal:decoding",
                "signed_directx" if normal_map_target == "unreal" else normal_map_target,
            )
            prim.SetCustomDataByKey(
                "blendkit:normal:outputConvention",
                "directx" if normal_map_target in ("unreal", "directx") else "opengl",
            )
            authored += 1
        elif shader_id == "UsdPreviewSurface":
            normal = shader.GetInput("normal")
            source = normal.GetConnectedSource() if normal else None
            if not source:
                continue
            texture = UsdShade.Shader(source[0].GetPrim())
            if not texture or texture.GetIdAttr().Get() != "UsdUVTexture":
                log(f"usd-normal: unsupported preview normal graph at {prim.GetPath()}; leaving unchanged")
                continue
            scale = texture.GetInput("scale")
            if not scale or not scale.GetAttr().HasAuthoredValueOpinion():
                scale = texture.CreateInput("scale", Sdf.ValueTypeNames.Float4)
                scale.Set(Gf.Vec4f(2, 2, 2, 1))
            bias = texture.GetInput("bias")
            if not bias or not bias.GetAttr().HasAuthoredValueOpinion():
                bias = texture.CreateInput("bias", Sdf.ValueTypeNames.Float4)
                bias.Set(Gf.Vec4f(-1, -1, -1, 0))
            texture.CreateInput("sourceColorSpace", Sdf.ValueTypeNames.Token).Set("raw")
            sy, by = scale.Get()[1], bias.Get()[1]
            convention = "opengl" if (sy, by) == (2, -1) else "directx" if (sy, by) == (-2, 1) else "unknown"
            texture.GetPrim().SetCustomDataByKey("blendkit:normal:inputConvention", convention)
            texture.GetPrim().SetCustomDataByKey("blendkit:normal:outputConvention", "opengl")
            file_input = texture.GetInput("file")
            file_path = file_input.Get() if file_input else None
            if file_path:
                texture.GetPrim().SetCustomDataByKey(
                    "blendkit:normal:sourceConvention",
                    source_conventions.get(os.path.basename(file_path.path), "unknown"),
                )
            authored += 1
    stage.GetRootLayer().Save()
    log(f"usd-normal: authored decoding metadata for {authored} shader(s)")
    return authored


def export_materialx_sidecar(out_usd: str, *, normal_map_target: str = "preserve") -> dict:
    """Export one ``.mtlx`` per used material next to *out_usd*.

    Layout (all relative to the .usd directory)::
        materials/<Material>.mtlx
        textures/<image files>       (shared with the .usd, see options below)

    Returns an in-memory mapping (Blender material name -> .mtlx path + status)
    that the caller uses to author the MaterialX reference layer.
    """
    out_dir = os.path.dirname(os.path.abspath(out_usd)) or "."
    mtlx_dirname = "materials"
    mtlx_dir = os.path.join(out_dir, mtlx_dirname)

    mapping: dict = {
        "usd": os.path.basename(out_usd),
        "generated_by": "export_usd.py",
        "materialx_available": False,
        "mtlx_dir": mtlx_dirname,
        "materials": [],
    }

    exporter = _load_mtlx_exporter()
    if exporter is None:
        if normal_map_target in ("directx", "unreal"):
            raise RuntimeError("Normal conversion requires the MaterialX exporter")
        return mapping

    mapping["materialx_available"] = True

    # A quiet logger — the exporter is extremely chatty at INFO level.
    import logging

    mtlx_logger = logging.getLogger("bk_mtlx_export")
    mtlx_logger.setLevel(logging.WARNING)
    if not mtlx_logger.handlers:
        h = logging.StreamHandler(stream=sys.stdout)
        h.setFormatter(logging.Formatter("[bk_mtlx] %(message)s"))
        mtlx_logger.addHandler(h)

    os.makedirs(mtlx_dir, exist_ok=True)

    options = {
        "export_textures": True,
        # Reuse the shared textures/ folder next to the .usd (populated by
        # unpack_textures) instead of copying a second set into materials/textures/.
        # The .mtlx materials reference ../textures/<file>, the same files the
        # UsdPreviewSurface network points at, so both networks stay in sync.
        "copy_textures": False,
        "texture_folder_name": "../textures",
        "materialx_version": "1.39",
        "active_uvmap": "UVMap",
        "strict_mode": False,  # one bad material must not abort the batch
        "optimize_document": True,
        "advanced_validation": False,
        "performance_monitoring": False,
    }

    materials = [m for m in bpy.data.materials if m.users > 0 and m.use_nodes]
    total = len(materials)
    log(f"mtlx: exporting {total} material(s) -> {mtlx_dir}")
    exported = 0
    for i, mat in enumerate(materials):
        fname = f"{_safe_filename(mat.name)}.mtlx"
        out_path = os.path.join(mtlx_dir, fname)
        entry: dict = {
            "name": mat.name,
            "mtlx_file": f"{mtlx_dirname}/{fname}",
            "success": False,
        }
        try:
            result = exporter(mat, out_path, mtlx_logger, options)
            entry["success"] = bool(result.get("success"))
            unsupported = result.get("unsupported_nodes") or []
            if unsupported:
                entry["unsupported_nodes"] = unsupported
            if result.get("error"):
                entry["error"] = str(result["error"])
            if entry["success"]:
                _sanitize_mtlx_names(out_path)
                _fix_mtlx_normal_maps(out_path)
                _fix_mtlx_scalar_connections(out_path)
                if normal_map_target == "directx":
                    _convert_mtlx_normal_maps_to_directx(out_path)
                elif normal_map_target == "unreal":
                    _prepare_mtlx_normals_for_unreal(out_path, material_name=mat.name)
                _repair_mtlx_types(out_path)
                invalid = _validate_mtlx(out_path)
                if invalid:
                    # Unreal falls back to usdMtlx's raw prim parse, so this degrades rather than fails.
                    entry["validation_errors"] = invalid
                    log(f"mtlx: INVALID {mat.name!r}: {invalid}")
                exported += 1
        except Exception as exc:  # never let one material abort the recipe
            entry["success"] = False
            entry["error"] = str(exc)
            log(f"mtlx: FAILED for {mat.name!r}: {exc}")
        mapping["materials"].append(entry)
        if total:
            progress(0.48 + 0.12 * (i + 1) / total, f"Exporting materials {i + 1}/{total}")

    log(f"mtlx: exported {exported}/{total} material(s) -> {mtlx_dir}")
    if normal_map_target == "unreal" and exported != total:
        failed = [
            f"{entry['name']!r} ({entry.get('error') or 'unknown error'})"
            for entry in mapping["materials"]
            if not entry["success"]
        ]
        raise RuntimeError(f"MaterialX export failed for {total - exported} material(s): {'; '.join(failed)}")
    return mapping


# ---------------------------------------------------------------------------
# MaterialX reference layer (wire the sidecar .mtlx into the USD)
# ---------------------------------------------------------------------------
# Blender's wm.usd_export can only emit its OWN inline MaterialX network. To
# make the USD use *our* richer bk_mtlx materials instead, we export geometry
# to a crate and author a thin ascii .usda that sublayers it and references
# each sidecar .mtlx onto its material prim under /root/_materials.

_VALID_USD_NAME = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_"
_SURFACEMATERIAL_RE = re.compile(r'<surfacematerial\s+name="([^"]+)"')


def _usd_safe_name(name: str) -> str:
    """Replicate Blender's USD prim-name sanitisation for a material.

    Any character outside ``[A-Za-z0-9_]`` becomes ``_`` and a leading digit
    is replaced with ``_`` — matching how Blender names each material prim
    under ``/root/_materials`` (e.g. ``steam.003`` -> ``steam_003``).
    """
    out = [ch if (ch in _VALID_USD_NAME and not (i == 0 and ch.isdigit())) else "_" for i, ch in enumerate(name)]
    return "".join(out) or "_"


def _read_mtlx_material_name(mtlx_path: str) -> str | None:
    """Return the ``<surfacematerial name=...>`` inside a .mtlx.

    usdMtlx exposes it at ``/MaterialX/Materials/<name>``; the name is often
    mangled by the exporter (e.g. ``steam_4``) so it must be read, not guessed.
    """
    try:
        with open(mtlx_path, encoding="utf-8") as fh:
            text = fh.read()
    except OSError:
        return None
    m = _SURFACEMATERIAL_RE.search(text)
    return m.group(1) if m else None


def write_mtlx_reference_layer(layer_path: str, geom_rel: str, mapping: dict) -> int:
    """Author *layer_path* as a text .usda composing the geometry crate + sidecar mtlx.

    The layer sublayers *geom_rel* (the exported <stem>.usd crate) and, for each
    material, overlays ``over /root/_materials/<Mat>`` with a reference to its
    ``materials/<Mat>.mtlx`` at ``</MaterialX/Materials/<surfacematerial>>``.

    Returns the number of materials wired.
    """
    out_dir = os.path.dirname(os.path.abspath(layer_path)) or "."
    body: list[str] = []
    wired = 0
    for entry in mapping.get("materials", []):
        if not entry.get("success"):
            continue
        mtlx_rel = entry.get("mtlx_file")  # e.g. "materials/steam_003.mtlx"
        if not mtlx_rel:
            continue
        surf = _read_mtlx_material_name(os.path.join(out_dir, mtlx_rel))
        if not surf:
            log(f"mtlx-ref: no <surfacematerial> in {mtlx_rel}; skipping")
            continue
        surf_path = _usd_safe_name(surf)
        prim = _usd_safe_name(entry.get("name", ""))
        body.append(f'        over "{prim}" (')
        body.append(f"            prepend references = @./{mtlx_rel}@</MaterialX/Materials/{surf_path}>")
        body.append("        )")
        body.append("        {")
        body.append("        }")
        wired += 1
        log(f"mtlx-ref: {prim} -> {mtlx_rel} </MaterialX/Materials/{surf_path}>")

    # Stage metadata (upAxis/metersPerUnit) is read only from the ROOT layer and
    # is NOT inherited from sublayers, so replicate the crate's Blender export
    # convention (Z-up, metres) here — otherwise consumers fall back to USD
    # defaults (Y-up, cm) and the asset loads rotated + 100x too small.
    lines = [
        "#usda 1.0",
        "(",
        '    defaultPrim = "root"',
        "    metersPerUnit = 1",
        '    upAxis = "Z"',
        "    subLayers = [",
        f"        @{geom_rel}@",
        "    ]",
        ")",
        "",
    ]
    if body:
        lines += ['over "root"', "{", '    over "_materials"', "    {", *body, "    }", "}", ""]
    with open(layer_path, "w", encoding="utf-8") as fh:
        fh.write("\n".join(lines))
    return wired


# ---------------------------------------------------------------------------
# Main export
# ---------------------------------------------------------------------------


def _prepare_material_asset(asset_name: str = "", asset_id: str = "") -> bool:
    """Ensure a material asset is bound to a visible mesh before export.

    Blendkit material ``.blend`` files contain the material datablock but
    frequently have **no mesh** using it (the addon appends the material and
    assigns it to a user-picked object at import time). ``wm.usd_export`` only
    writes materials that are bound to exported geometry, so without this step
    the USD comes out empty and the Maya side reports "no material found".

    We locate the asset material and, if no visible mesh already uses it,
    create a simple plane and assign it. The plane's geometry/UVs are
    irrelevant: the Maya side only lifts the resulting ``shadingEngine`` and
    assigns it to the real target mesh, discarding the preview geometry.

    Returns True when a material was found (and bound), False otherwise.
    """
    mats = list(bpy.data.materials)
    if not mats:
        log("material asset: no materials in blend")
        return False

    # Pick the asset material: prefer one marked as an asset, then match by
    # Blendkit id, then by name, finally fall back to the first material.
    chosen = None
    for m in mats:
        if getattr(m, "asset_data", None) is not None:
            chosen = m
            break
    if chosen is None and asset_id:
        for m in mats:
            # use new var first, then fallback to old-style blenderkit
            bk = getattr(m, "blendkit", None)
            if bk is None:
                bk = getattr(m, "blenderkit", None)
            if bk is not None and getattr(bk, "id", "") == asset_id:
                chosen = m
                break
    if chosen is None and asset_name:
        for m in mats:
            if m.name == asset_name:
                chosen = m
                break
    if chosen is None:
        chosen = mats[0]
    log(f"material asset: chosen material = {chosen.name!r}")

    # Already bound to a visible mesh? Then there's nothing to do.
    for obj in bpy.context.scene.objects:
        if obj.type != "MESH" or obj.data is None:
            continue
        if any(slot and slot.name == chosen.name for slot in obj.data.materials):
            log(f"material asset: already on mesh {obj.name!r}")
            return True

    # Build a unit plane and assign the material.
    mesh = bpy.data.meshes.new(f"{chosen.name}_preview")
    verts = [(-1.0, -1.0, 0.0), (1.0, -1.0, 0.0), (1.0, 1.0, 0.0), (-1.0, 1.0, 0.0)]
    faces = [(0, 1, 2, 3)]
    mesh.from_pydata(verts, [], faces)
    mesh.update()
    # A 0..1 UV map so any image textures resolve during export.
    uv = mesh.uv_layers.new(name="UVMap")
    uv_coords = [(0.0, 0.0), (1.0, 0.0), (1.0, 1.0), (0.0, 1.0)]
    for loop in mesh.loops:
        uv.data[loop.index].uv = uv_coords[loop.vertex_index]
    mesh.materials.append(chosen)

    obj = bpy.data.objects.new(f"{chosen.name}_preview", mesh)
    bpy.context.scene.collection.objects.link(obj)
    log(f"material asset: created preview plane for {chosen.name!r}")
    return True


def _flatten_rig_to_single_root(group_name: str = "") -> int:
    """Drop a dangling armature rig and gather parts under one model root.

    BlenderKit "rigged" props (e.g. the daewoo truck) rig their parts by rigid
    bone-parenting, not skin weights: Blender exports an unbound Skeleton +
    SkelAnimation (no SkelRoot, 0 skinned meshes) plus each part as its own
    posed Xform. Unreal/Maya then import that as hundreds of loose pieces.

    Each part is already at its correct world position, so we detach the parts
    from the armature (preserving world transforms), delete the orphan
    skeleton, and reparent every top-level object under a single empty. The USD
    then has /root/<Model>/<parts> — one asset, parts still individually
    movable, positions 1:1. No-op when there is no armature.

    Returns the number of armature objects removed.
    """
    armatures = [o for o in bpy.data.objects if o.type == "ARMATURE"]
    if not armatures:
        return 0

    objs = [o for o in bpy.context.scene.objects if o.type != "ARMATURE"]
    worlds = {o.name: o.matrix_world.copy() for o in objs}

    # Detach parts parented to the rig (object- or bone-parented) in place.
    for o in objs:
        if o.parent in armatures:
            o.parent = None
            o.matrix_world = worlds[o.name]

    removed = 0
    for arm in armatures:
        with contextlib.suppress(Exception):
            bpy.data.objects.remove(arm, do_unlink=True)
            removed += 1

    name = group_name or os.path.splitext(os.path.basename(bpy.data.filepath))[0] or "Model"
    group = bpy.data.objects.new(name, None)
    bpy.context.scene.collection.objects.link(group)
    grouped = 0
    for o in list(bpy.context.scene.objects):
        if o is group or o.parent is not None:
            continue
        world = o.matrix_world.copy()
        o.parent = group
        o.matrix_parent_inverse = group.matrix_world.inverted()
        o.matrix_world = world
        grouped += 1

    log(f"rig-flatten: removed {removed} armature(s), grouped {grouped} root object(s) under {name!r}")
    return removed


def export_to_usd(
    blend_path: str,
    out_usd: str,
    asset_type: str = "model",
    asset_name: str = "",
    asset_id: str = "",
    *,
    export_mtlx: bool = True,
    mtlx_references: bool = True,
    normal_map_target: str = "preserve",
    reload_blend: bool = True,
) -> str:
    """Export a .blend to .usd with the necessary pre-processing for Maya compatibility.

    Args:
      blend_path: path to the source .blend file to export
      out_usd: path to write the resulting .usd file
      asset_type: Blendkit asset type ("material" gets a preview-mesh step)
      asset_name: asset display name (used to locate the asset material)
      asset_id: Blendkit asset id (used to locate the asset material)
      export_mtlx: also emit standalone .mtlx materials referenced by the .usd
      mtlx_references: when the sidecar .mtlx exist, make the USD reference them
        (geometry goes to the <stem>.usd crate + a thin <stem>.usda composes it)
        instead of Blender's inline MaterialX network
      normal_map_target: "directx" flips tangent-space normal-map green channels
        in the MaterialX sidecars; "preserve" leaves their convention unchanged;
        "unreal" explicitly decodes raw samples into signed DirectX vectors
      reload_blend: open *blend_path* first; pass False only when Blender
        already started with that file and nothing has modified it yet

    Returns:
      The path Maya should load: the <stem>.usda wrapper when MaterialX
      references were written, otherwise the plain out_usd crate.

    Raises:
      ValueError: If the target or its required MaterialX options are invalid.
      RuntimeError: If the required Unreal MaterialX export fails.
    """
    if normal_map_target not in ("preserve", "directx", "unreal"):
        raise ValueError(f"Unsupported normal_map_target: {normal_map_target!r}")
    if normal_map_target == "unreal" and (not export_mtlx or not mtlx_references):
        raise ValueError("Unreal normal decoding requires export_mtlx and mtlx_references")

    status("Opening blend")
    if reload_blend:
        bpy.ops.wm.open_mainfile(filepath=blend_path)
    progress(0.05, "Opened blend")

    # Material assets carry the material datablock but usually no mesh — bind
    # it to a preview plane so wm.usd_export actually writes the material.
    if asset_type.lower() == "material":
        status("Preparing material")
        if not _prepare_material_asset(asset_name, asset_id):
            log("material asset: no material to bind (export may be empty)")

    # Make everything visible so the export captures the full asset.
    for obj in bpy.context.scene.objects:
        with contextlib.suppress(Exception):
            obj.hide_set(False)
        obj.hide_render = False
        obj.hide_viewport = False

    status("Consolidating rig")
    _flatten_rig_to_single_root(asset_name)
    progress(0.08, "Consolidated rig")

    status("Unpacking textures")
    unpack_textures(blend_path, progress_range=(0.08, 0.30))
    progress(0.30, "Unpacked textures")

    status("Flattening materials")
    _flatten_material_node_groups()
    progress(0.45, "Flattened materials")

    if normal_map_target == "unreal":
        for material in bpy.data.materials:
            if re.search(r"_\d+$", _usd_safe_name(material.name)):
                name = f"{material.name}_Material"
                while bpy.data.materials.get(name) is not None:
                    name += "_"
                # Unreal's MaterialX cache uses FName's plain name, which drops a numeric suffix.
                material.name = name

    status("Renaming shader nodes")
    _rename_shader_nodes_to_material()
    progress(0.48, "Renamed shader nodes")

    # Emit standalone MaterialX materials while the material graphs are still
    # intact (the destructive split/force-single passes below drop material
    # slots). Skipped gracefully if MaterialX is unavailable.
    mtlx_mapping: dict | None = None
    if export_mtlx:
        status("Exporting MaterialX")
        try:
            mtlx_mapping = export_materialx_sidecar(out_usd, normal_map_target=normal_map_target)
        except Exception as exc:
            if normal_map_target == "unreal":
                raise RuntimeError(f"Required Unreal MaterialX export failed: {exc}") from exc
            log(f"mtlx: sidecar export failed (non-fatal): {exc}")
        progress(0.60, "MaterialX exported")

    # NOTE: intentionally do NOT re-save the .blend here. unpack_textures()
    # already wrote the images to disk and repointed the in-memory image
    # datablocks, so wm.usd_export (same session) resolves them without a save.
    # Overwriting the original download would re-encode it with this (newer)
    # Blender and break re-imports in older Blender versions of the addon.

    status("Splitting multi-material meshes")
    _split_meshes_by_material()
    progress(0.62, "Split materials")

    status("Forcing single material per mesh")
    _force_single_material_per_mesh()
    progress(0.64, "Single-material per mesh")

    status("Removing empty UV layers")
    _remove_empty_uv_layers()
    progress(0.65, "Removed empty UV layers")

    status("Fixing primary UV set")
    _fix_primary_uv_set()
    progress(0.66, "Primary UV set fixed")

    status("Sanitizing meshes for USD export")
    _sanitize_meshes_for_usd_export()
    progress(0.68, "Sanitized meshes")

    status("Generating USD")
    out_dir = os.path.dirname(out_usd) or "."
    os.makedirs(out_dir, exist_ok=True)

    # Wire our sidecar .mtlx as the MaterialX source when they exist. Geometry
    # goes to the <stem>.usd crate and Blender's inline MaterialX network is
    # dropped (a referenced material can't override an inline one); a thin
    # <stem>.usda composes the crate + references our .mtlx and becomes the
    # file Maya loads. Otherwise export straight to out_usd with inline mtlx.
    use_refs = bool(
        mtlx_references and mtlx_mapping and any(e.get("success") for e in mtlx_mapping.get("materials", []))
    )
    geom_rel = f"./{os.path.basename(out_usd)}"
    wrapper_usda = os.path.splitext(out_usd)[0] + ".usda"

    desired = {
        # Geometry (and, when not referencing our sidecars, inline MaterialX)
        # always lands in the <stem>.usd crate.
        "filepath": out_usd,
        "selected_objects_only": False,
        "visible_objects_only": True,
        "export_animation": True,
        "export_hair": True,
        "export_uvmaps": True,
        "export_normals": True,
        "export_materials": True,
        # Textures are already on disk via our unpack step — no need to
        # copy them again. This also keeps the USD asset-paths pointing at
        # the resolution-specific ``textures_2k/`` next to the blend.
        "export_textures": False,
        "overwrite_textures": False,
        "relative_paths": True,
        "generate_preview_surface": True,
        # Inline MaterialX only when we are NOT referencing our own sidecar
        # .mtlx; UsdPreviewSurface stays as the universal-context fallback.
        "generate_materialx_network": not use_refs,
        "root_prim_path": "/root",
        "default_prim_path": "/root",
        "export_global_forward_selection": "Y",
        "export_global_up_selection": "Z",
    }
    try:
        rna_props = bpy.ops.wm.usd_export.get_rna_type().properties
        rna = set(rna_props.keys())
    except Exception as exc:
        log(f"usd_export: rna introspection failed ({exc}); passing all kwargs")
        rna = set(desired)

    accepted = {k: v for k, v in desired.items() if k in rna}
    skipped = sorted(set(desired) - set(accepted))
    if skipped:
        log(f"usd_export: skipped unknown kwargs = {skipped}")
    # Loud diagnostic: confirm what we actually pass for texture-handling.
    tex_flags = {k: v for k, v in accepted.items() if "texture" in k.lower() or "relative" in k.lower()}
    log(f"usd_export: texture-related kwargs accepted = {tex_flags}")

    progress(0.70, "Generating USD")
    bpy.ops.wm.usd_export(**accepted)
    progress(0.90, "Exported USD")

    primary = out_usd
    if use_refs:
        status("Linking MaterialX materials")
        wired = write_mtlx_reference_layer(wrapper_usda, geom_rel, mtlx_mapping)
        log(f"mtlx-ref: wired {wired} material(s) into {os.path.basename(wrapper_usda)}")
        # The .usd crate is now a sub-layer of the .usda wrapper Maya loads.
        artifact(out_usd)
        primary = wrapper_usda
    progress(0.93, "Writing normal metadata")
    _author_usd_normal_metadata(primary, normal_map_target)
    progress(0.99, "Exported USD")
    return primary


# ---------------------------------------------------------------------------
# Entry point
# ---------------------------------------------------------------------------


def _parse_args() -> dict:
    if "--" not in sys.argv:
        raise RuntimeError("missing '--' separator in argv")
    args = sys.argv[sys.argv.index("--") + 1 :]
    if not args:
        raise RuntimeError("missing args JSON path after '--'")
    with open(args[-1], encoding="utf-8") as fh:
        return json.load(fh)


def main() -> int:
    """Main entry point when run as a Blender script.

    Expects a single JSON file argument with the following keys:
      - blend_path: path to the source .blend file to export
      - out_usd: path to write the resulting .usd file
      - max_resolution: optional max texture resolution (e.g. "2048")
    """
    try:
        params = _parse_args()
    except Exception as exc:
        error(f"argument parsing failed: {exc}")
        return 2

    blend_path = params.get("blend_path") or ""
    out_usd = params.get("out_usd") or ""

    if not blend_path or not os.path.isfile(blend_path):
        error(f"blend_path not found: {blend_path!r}")
        return 1
    if not out_usd:
        error("out_usd not provided")
        return 1

    try:
        # Callers usually launch Blender with the .blend already open; reopening costs seconds on large files.
        already_open = bool(bpy.data.filepath) and os.path.normcase(
            os.path.abspath(bpy.data.filepath)
        ) == os.path.normcase(os.path.abspath(blend_path))
        primary = export_to_usd(
            blend_path,
            out_usd,
            asset_type=str(params.get("asset_type") or "model"),
            asset_name=str(params.get("asset_name") or ""),
            asset_id=str(params.get("asset_id") or ""),
            export_mtlx=bool(params.get("export_mtlx", True)),
            mtlx_references=bool(params.get("mtlx_references", True)),
            normal_map_target=str(params.get("normal_map_target") or "preserve"),
            reload_blend=not already_open,
        )
    except Exception as exc:
        error(f"export failed: {exc}")
        traceback.print_exc(file=sys.stdout)
        # Callers cache by output existence; a half-written stage must not look finished.
        for partial in (out_usd, os.path.splitext(out_usd)[0] + ".usda"):
            with contextlib.suppress(OSError):
                os.remove(partial)
        return 1

    progress(1.0, "done")
    done(primary or out_usd)
    return 0


if __name__ == "__main__":
    sys.exit(main())
