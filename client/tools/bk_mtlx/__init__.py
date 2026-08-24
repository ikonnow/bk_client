"""Vendored, pure-Python MaterialX exporter (from the bl_mtlx add-on).

Bundled inside the Blendkit client so the ``export_usd`` recipe can emit
standalone ``.mtlx`` materials without requiring the add-on to be installed
or enabled in Blender. Loaded under this unique package name (``bk_mtlx``)
so it never clashes with the ``material_x`` extension if it is also enabled.

Only the modules on the export code path are vendored:
    blender_materialx_exporter, materialx_library_core,
    mtlxutils/{mxbase, mxfile, mxnodegraph, mxtraversal}

The compiled ``MaterialX`` (PyMaterialX) package is NOT vendored — it ships
with Blender 4.1+. Callers must guard the import and degrade gracefully.
"""
