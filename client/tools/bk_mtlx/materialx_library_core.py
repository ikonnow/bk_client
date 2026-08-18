#!/usr/bin/env python3
"""MaterialX Library Core - Phase 1, 2 & 3 Implementation.

This module implements the core infrastructure migration from manual XML generation
to MaterialX library APIs as outlined in the integration plan.

Phase 1: Core Infrastructure Migration
- Replace XML generation with MaterialX library
- Implement library loading system
- Create MaterialX document builder

Phase 2: Node Mapping System Enhancement
- Leverage node definitions for type-safe mapping
- Implement automatic type conversion and validation
- Enhanced node creation with validation

Phase 3: Advanced Features Integration
- File writing improvements with advanced options
- Enhanced validation and error handling
- Performance optimization and memory management
- Error recovery and robustness features
"""

from __future__ import annotations

import gc
import os
import time
from typing import Any

import MaterialX as mx  # type: ignore # noqa: N813

# Import mtlxutils
try:
    from . import mtlxutils  # type: ignore # noqa: F401
    from .mtlxutils import mxbase as mxb  # noqa: F401
    from .mtlxutils import mxfile as mxf
    from .mtlxutils import mxnodegraph as mxg  # noqa: F401
    from .mtlxutils import mxtraversal as mxt  # noqa: F401

except ImportError:
    # Fallback for direct import
    import mtlxutils.mxfile as mxf


class MaterialXConfig:
    """Configuration system for MaterialX export settings."""

    # Default configuration
    DEFAULT_CONFIG = {  # noqa: RUF012
        # Core settings
        "materialx_version": "1.39",
        "optimize_document": True,
        "advanced_validation": True,
        "performance_monitoring": True,
        # File writing options
        "skip_library_elements": True,
        "write_xinclude": False,
        "remove_layout": True,
        "format_output": True,
        # Error handling
        "strict_mode": True,
        "max_errors": 10,
        # Performance settings
        "enable_caching": True,
        "cache_size_limit": 1000,
        "memory_warning_threshold_mb": 100,
        # Validation settings
        "validate_connections": True,
        "check_circular_dependencies": True,
        "warn_unused_nodes": True,
        "max_node_count_warning": 100,
        "max_nesting_depth_warning": 10,
    }

    def __init__(self, custom_config: dict | None = None):
        """Initialize configuration with defaults and custom overrides."""
        self.config = self.DEFAULT_CONFIG.copy()
        if custom_config:
            self.config.update(custom_config)

    def get(self, key: str, default=None):
        """Get configuration value."""
        return self.config.get(key, default)

    def set(self, key: str, value):
        """Set configuration value."""
        self.config[key] = value

    def update(self, new_config: dict):
        """Update configuration with new values."""
        self.config.update(new_config)

    def get_export_options(self) -> dict:
        """Get export-specific options."""
        return {
            "optimize_document": self.get("optimize_document"),
            "advanced_validation": self.get("advanced_validation"),
            "performance_monitoring": self.get("performance_monitoring"),
        }

    def get_write_options(self) -> dict:
        """Get file writing options."""
        return {
            "skip_library_elements": self.get("skip_library_elements"),
            "write_xinclude": self.get("write_xinclude"),
            "remove_layout": self.get("remove_layout"),
            "format_output": self.get("format_output"),
        }


class MaterialXValidationError(Exception):
    """Custom exception for MaterialX validation errors."""


class MaterialXError(Exception):
    """Base class for MaterialX-specific errors with classification."""

    def __init__(self, message: str, error_type: str = "general", details: dict | None = None):
        super().__init__(message)
        self.error_type = error_type
        self.details = details or {}

    def get_user_friendly_message(self) -> str:
        """Get a user-friendly error message."""
        error_messages = {
            "library_loading": "Failed to load MaterialX libraries. Please check your MaterialX installation.",
            "node_creation": "Failed to create MaterialX node. The node type may not be supported.",
            "connection_error": "Failed to connect nodes. There may be a type mismatch.",
            "validation_error": "MaterialX document validation failed. Check the material setup.",
            "file_write": "Failed to write MaterialX file. Check file permissions and disk space.",
            "type_conversion": "Failed to convert data types. Check input values.",
            "unsupported_node": "This Blender node type is not supported in MaterialX export.",
            "performance_warning": "Export completed but performance issues were detected.",
            "memory_error": "Insufficient memory for export operation.",
        }
        return error_messages.get(self.error_type, str(self))


class MaterialXPerformanceMonitor:
    """Monitors performance metrics for MaterialX operations.

    This class provides:
    - Operation timing
    - Memory usage tracking
    - Performance optimization suggestions
    - Resource cleanup monitoring
    """

    def __init__(self, logger):
        self.logger = logger
        self.operation_times = {}
        self.memory_snapshots = {}
        self.start_time = None
        self.enabled = True

    def start_operation(self, operation_name: str):
        """Start timing an operation."""
        if not self.enabled:
            return

        self.operation_times[operation_name] = {"start": time.time(), "memory_before": self._get_memory_usage()}

    def end_operation(self, operation_name: str):
        """End timing an operation and log results."""
        if not self.enabled or operation_name not in self.operation_times:
            return

        end_time = time.time()
        memory_after = self._get_memory_usage()

        timing = self.operation_times[operation_name]
        duration = end_time - timing["start"]
        memory_delta = memory_after - timing["memory_before"]

        self.logger.debug("Operation '%s': %.4fs, Memory: %+d bytes", operation_name, duration, memory_delta)

        # Performance warnings
        if duration > 1.0:
            self.logger.warning("Slow operation detected: '%s' took %.4fs", operation_name, duration)
        if memory_delta > 10 * 1024 * 1024:  # 10MB
            self.logger.warning(
                "High memory usage detected: '%s' used %.2fMB", operation_name, memory_delta / (1024 * 1024)
            )

    def _get_memory_usage(self) -> int:
        """Get current memory usage in bytes."""
        try:
            import psutil  # type: ignore

            process = psutil.Process()
            return process.memory_info().rss
        except ImportError:
            return 0

    def suggest_optimizations(self) -> list[str]:
        """Analyze performance data and suggest optimizations."""
        suggestions = []

        if not self.operation_times:
            return suggestions

        # Find slowest operations
        slow_operations = [(name, data.get("duration", 0)) for name, data in self.operation_times.items()]
        slow_operations.sort(key=lambda x: x[1], reverse=True)

        if slow_operations and slow_operations[0][1] > 0.5:
            suggestions.append(f"Consider optimizing '{slow_operations[0][0]}' (took {slow_operations[0][1]:.4f}s)")

        return suggestions

    def cleanup(self):
        """Clean up performance monitoring data."""
        self.operation_times.clear()
        self.memory_snapshots.clear()
        gc.collect()


class MaterialXAdvancedValidator:
    """Advanced validation features for MaterialX documents.

    This class provides:
    - Comprehensive document validation
    - Node definition validation
    - Connection validation
    - Performance validation
    - Custom validation rules
    """

    def __init__(self, logger):
        self.logger = logger
        self.validation_rules = {}
        self.custom_validators = {}

    def validate_document_comprehensive(self, document: mx.Document) -> dict[str, Any]:
        """Perform comprehensive validation of a MaterialX document.

        Args:
            document: The MaterialX document to validate

        Returns:
            Dict containing validation results
        """
        results = {"valid": True, "warnings": [], "errors": [], "performance_issues": [], "suggestions": []}

        try:
            # Basic document validation
            if not self._validate_basic_structure(document, results):
                results["valid"] = False

            # Node validation
            self._validate_nodes(document, results)

            # Connection validation
            self._validate_connections(document, results)

            # Performance validation
            self._validate_performance(document, results)

            # Custom validation rules
            self._apply_custom_validators(document, results)

        except Exception as e:
            results["valid"] = False
            results["errors"].append(f"Validation failed with exception: {e!s}")
            self.logger.exception("Document validation failed:")

        return results

    def _validate_basic_structure(self, document: mx.Document, results: dict[str, Any]) -> bool:
        """Validate basic document structure."""
        try:
            # Check for required elements
            materials = document.getMaterialNodes()
            if not materials:
                results["warnings"].append("No material nodes found in document")

            # Check for surface shaders - get all nodes and filter by type
            all_nodes = document.getNodes()
            surface_shaders = [node for node in all_nodes if node.getType() == "surfaceshader"]
            if not surface_shaders:
                results["warnings"].append("No surface shader nodes found in document")

            # Check for nodegraphs
            nodegraphs = document.getNodeGraphs()
            if not nodegraphs:
                results["warnings"].append("No nodegraphs found in document")

        except Exception as e:
            results["errors"].append(f"Basic structure validation failed: {e!s}")
            return False
        else:
            return True

    def _validate_nodes(self, document: mx.Document, results: dict[str, Any]):
        """Validate all nodes in the document."""
        try:
            nodes = document.getNodes()
            for node in nodes:
                # Check node definition
                nodedef = node.getNodeDef()
                if not nodedef:
                    results["warnings"].append(f"Node '{node.getName()}' has no node definition")
                    continue

                # Check input connections - use MaterialX 1.39 compatible API
                for input_elem in node.getInputs():
                    try:
                        # Check if input has a connection
                        if hasattr(input_elem, "getConnectedNode"):
                            connected_node = input_elem.getConnectedNode()
                            if connected_node and not document.getChild(connected_node.getName()):
                                # Validate the connected node exists
                                results["warnings"].append(
                                    f"Input '{input_elem.getName()}' on node '{node.getName()}' connects to non-existent node"
                                )
                    except Exception:
                        self.logger.exception("Error validating input connections for node '%s'", node.getName())

                # Check output connections - use MaterialX 1.39 compatible API
                for output_elem in node.getOutputs():
                    try:
                        # Check if output has connections
                        if hasattr(output_elem, "getConnections"):
                            connections = output_elem.getConnections()
                            if connections:
                                for connection in connections:
                                    if hasattr(connection, "getDownstreamElement"):
                                        downstream = connection.getDownstreamElement()
                                        if downstream and not document.getChild(downstream.getName()):
                                            results["warnings"].append(
                                                f"Output '{output_elem.getName()}' on node '{node.getName()}' connects to non-existent node"
                                            )
                    except Exception:
                        # Skip validation for this output if API is not available
                        self.logger.exception("Error validating output connections for node '%s'", node.getName())

        except Exception as e:
            results["errors"].append(f"Node validation failed: {e!s}")

    def _validate_connections(self, document: mx.Document, results: dict[str, Any]):
        """Validate all connections in the document."""
        try:
            # Use MaterialX 1.39 compatible connection analysis
            nodes = document.getNodes()
            for node in nodes:
                # Check input connections
                for input_elem in node.getInputs():
                    try:
                        if hasattr(input_elem, "getConnectedNode"):
                            connected_node = input_elem.getConnectedNode()
                            if connected_node and not document.getChild(connected_node.getName()):
                                results["warnings"].append(
                                    f"Input '{input_elem.getName()}' on node '{node.getName()}' connects to non-existent node"
                                )
                    except Exception:
                        # Skip validation if API is not available
                        self.logger.exception("Error validating input connections for node '%s'", node.getName())

        except Exception as e:
            results["errors"].append(f"Connection validation failed: {e!s}")

    def _validate_performance(self, document: mx.Document, results: dict[str, Any]):
        """Validate document for performance issues."""
        try:
            # Check for excessive node count
            nodes = document.getNodes()
            if len(nodes) > 100:
                results["performance_issues"].append(f"Large number of nodes ({len(nodes)}) may impact performance")

            # Check for deep nesting
            nodegraphs = document.getNodeGraphs()
            for nodegraph in nodegraphs:
                depth = self._calculate_nesting_depth(nodegraph)
                if depth > 5:
                    results["performance_issues"].append(
                        f"Deep nesting detected in nodegraph '{nodegraph.getName()}' (depth: {depth})"
                    )

            # Check for unused nodes
            unused_nodes = self._find_unused_nodes(document)
            if unused_nodes:
                results["suggestions"].append(f"Found {len(unused_nodes)} unused nodes that could be removed")

        except Exception as e:
            results["errors"].append(f"Performance validation failed: {e!s}")

    def _has_circular_connection(self, document: mx.Document, start_elem: mx.Element, target_elem: mx.Element) -> bool:
        """Check for circular connections."""
        visited = set()
        return self._dfs_for_circular(start_elem, target_elem, visited, document)

    def _dfs_for_circular(self, current: mx.Element, target: mx.Element, visited: set, document: mx.Document) -> bool:
        """Depth-first search for circular connections."""
        if current == target:
            return True

        if current.getNamePath() in visited:
            return False

        visited.add(current.getNamePath())

        # Check downstream connections
        if current.isA(mx.Node):
            for output in current.getOutputs():
                if output.isConnected():
                    for edge in output.getConnections():
                        downstream = edge.getDownstreamElement()
                        if downstream and self._dfs_for_circular(downstream, target, visited, document):
                            return True

        return False

    def _calculate_nesting_depth(self, element: mx.Element, current_depth: int = 0) -> int:
        """Calculate the nesting depth of an element."""
        max_depth = current_depth

        for child in element.getChildren():
            if child.isA(mx.NodeGraph):
                child_depth = self._calculate_nesting_depth(child, current_depth + 1)
                max_depth = max(max_depth, child_depth)

        return max_depth

    def _find_unused_nodes(self, document: mx.Document) -> list[mx.Node]:
        """Find nodes that are not connected to any material output."""
        try:
            # Get all nodes
            all_nodes = list(document.getNodes())

            # Find nodes connected to materials
            connected_nodes = []
            materials = document.getMaterialNodes()

            for material in materials:
                self._collect_connected_nodes(material, connected_nodes, document)

            # Return unused nodes (nodes not in connected_nodes)
            unused_nodes = [n for n in all_nodes if n not in connected_nodes]

        except Exception:
            self.logger.exception("Error finding unused nodes")
            return []

        else:
            self.logger.info("Found %s unused nodes in the document", len(unused_nodes))
            return unused_nodes

    def _collect_connected_nodes(self, element: mx.Element, connected_nodes: list, document: mx.Document):
        """Collect all nodes connected to a given element."""
        if element.isA(mx.Node):
            if element not in connected_nodes:
                connected_nodes.append(element)

            # Check inputs
            for input_elem in element.getInputs():
                try:
                    if hasattr(input_elem, "getConnectedNode"):
                        connected_node = input_elem.getConnectedNode()
                        if connected_node and connected_node not in connected_nodes:
                            self._collect_connected_nodes(connected_node, connected_nodes, document)
                except Exception:
                    # Skip if API is not available
                    self.logger.exception(
                        "Error collecting connected nodes for input '%s' on node '%s'",
                        input_elem.getName(),
                        element.getName(),
                    )

    def _apply_custom_validators(self, document: mx.Document, results: dict[str, Any]):
        """Apply custom validation rules."""
        for validator_name, validator_func in self.custom_validators.items():
            try:
                validator_result = validator_func(document)
                if not validator_result["valid"]:
                    results["errors"].extend(validator_result.get("errors", []))
                    results["warnings"].extend(validator_result.get("warnings", []))
            except Exception as e:
                results["errors"].append(f"Custom validator '{validator_name}' failed: {e!s}")

    def add_custom_validator(self, name: str, validator_func):
        """Add a custom validation rule."""
        self.custom_validators[name] = validator_func

    def get_validation_summary(self, results: dict[str, Any]) -> str:
        """Get a summary of validation results."""
        summary = f"Validation {'PASSED' if results['valid'] else 'FAILED'}\n"
        summary += f"Errors: {len(results['errors'])}\n"
        summary += f"Warnings: {len(results['warnings'])}\n"
        summary += f"Performance Issues: {len(results['performance_issues'])}\n"
        summary += f"Suggestions: {len(results['suggestions'])}"
        return summary


class MaterialXDocumentManager:
    """Manages MaterialX document creation and library loading.

    This class handles:
    - MaterialX document creation
    - Library loading and validation
    - Version compatibility
    - Search path management
    - Advanced validation and error recovery
    - Performance monitoring
    """

    def __init__(self, logger, version: str = "1.39"):
        self.logger = logger
        self.version = version
        self.document = None
        self.libraries = None
        self.library_files = []
        self.search_path = None

        # Phase 3 enhancements
        self.performance_monitor = MaterialXPerformanceMonitor(logger)
        self.advanced_validator = MaterialXAdvancedValidator(logger)

        # Cache for performance optimization
        self._node_def_cache = {}
        self._input_def_cache = {}
        self._output_def_cache = {}

    def load_libraries(self, custom_search_path: str | None = None) -> bool:  # noqa: ARG002
        """Load MaterialX libraries with proper version handling.

        Args:
            custom_search_path: Optional custom search path for libraries

        Returns:
            bool: True if libraries loaded successfully
        """
        try:
            self.performance_monitor.start_operation("load_libraries")

            self.logger.info("Loading MaterialX libraries (version: %s)", self.version)

            # Create libraries document if not already created
            if self.libraries is None:
                self.libraries = mx.createDocument()

            # Use the working method from our debug test
            self.logger.info("Using MaterialX 1.39+ library loading method")
            search_path = mx.getDefaultDataSearchPath()
            lib_folders = mx.getDefaultDataLibraryFolders()
            self.library_files = mx.loadLibraries(lib_folders, search_path, self.libraries)

            self.logger.info("Loaded %s library files", len(self.library_files))

            # Clear caches after library loading
            self._clear_caches()

            self.performance_monitor.end_operation("load_libraries")

        except Exception:
            self.logger.exception("Failed to load MaterialX libraries")
            self.performance_monitor.end_operation("load_libraries")
            return False

        else:
            return True

    def create_document(self) -> mx.Document:
        """Create a new MaterialX document with loaded libraries and validation.

        Returns:
            mx.Document: The created document
        """
        try:
            self.performance_monitor.start_operation("create_document")

            self.logger.info("Creating MaterialX document")

            # Create libraries document if not already created
            if self.libraries is None:
                self.libraries = mx.createDocument()
                if not self.load_libraries():
                    raise RuntimeError("Failed to load MaterialX libraries")  # noqa: TRY301

            # Create working document
            self.document = mx.createDocument()

            # Set colorspace attribute (required for MaterialX compliance)
            self.document.setColorSpace("lin_rec709")

            self.logger.info("Working document has %s node definitions before import", len(self.document.getNodeDefs()))
            self.document.importLibrary(self.libraries)
            self.logger.info("Working document has %s node definitions after import", len(self.document.getNodeDefs()))

            # Validate document after creation
            validation_results = self.advanced_validator.validate_document_comprehensive(self.document)
            if not validation_results["valid"]:
                self.logger.warning("Document validation issues detected:")
                for error in validation_results["errors"]:
                    self.logger.warning("  - %s", error)

            self.logger.info("MaterialX document created successfully")

            self.performance_monitor.end_operation("create_document")

        except Exception:
            self.logger.exception("Failed to create MaterialX document")
            self.performance_monitor.end_operation("create_document")
            raise

        else:
            return self.document

    def get_node_definition(self, node_type: str, category: str | None = None) -> mx.NodeDef | None:
        """Get a node definition from the loaded libraries with caching.

        Args:
            node_type: The node type to find
            category: Optional category filter

        Returns:
            mx.NodeDef: The node definition or None if not found
        """
        if not self.document:
            self.logger.error("No document available for node definition lookup")
            return None

        # Create cache key
        cache_key = f"{node_type}_{category or 'any'}"

        # Check cache first
        if cache_key in self._node_def_cache:
            return self._node_def_cache[cache_key]

        try:
            self.performance_monitor.start_operation("get_node_definition")

            # Get all node definitions and search through them
            all_node_defs = self.document.getNodeDefs()
            self.logger.debug(
                "DEBUG: Searching for node definition '%s' (category: %s) among %d node definitions",
                node_type,
                category,
                len(all_node_defs),
            )
            self.logger.info(
                "Searching for node definition '%s' (category: %s) among %d node definitions",
                node_type,
                category,
                len(all_node_defs),
            )

            # Look for exact match first by node type
            for nodedef in all_node_defs:
                if nodedef.getType() == node_type and (category is None or nodedef.getCategory() == category):
                    result = nodedef
                    self.logger.info("Found exact match by type: %s", nodedef.getName())
                    break
            else:
                # If no exact match by type, try searching by node name
                print("DEBUG: No exact match by type, trying search by name...")
                self.logger.info("No exact match by type, trying search by name...")

                # Debug: Show some node names that contain our search term
                matching_names = []
                for nodedef in all_node_defs:
                    nodedef_name = nodedef.getName()
                    if node_type.lower() in nodedef_name.lower():
                        matching_names.append(nodedef_name)

                if matching_names:
                    self.logger.debug(
                        "Found %d node names containing '%s': %s", len(matching_names), node_type, matching_names[:5]
                    )

                # Collect all name matches, then prefer an exact standard nodedef over
                # gltf_* variants: Unreal's standard_surface pipeline expects standard nodes.
                name_matches = [
                    nd
                    for nd in all_node_defs
                    if node_type.lower() in nd.getName().lower() and (category is None or nd.getType() == category)
                ]

                if name_matches:
                    want_gltf = "gltf" in node_type.lower()
                    exact_name = f"nd_{node_type.lower()}_{(category or '').lower()}"
                    result = sorted(
                        name_matches,
                        key=lambda nd: (
                            nd.getName().lower() != exact_name,
                            ("gltf" in nd.getName().lower()) and not want_gltf,
                        ),
                    )[0]
                    self.logger.debug("Found match by name: %s (type: %s)", result.getName(), result.getType())
                    self.logger.info("Found match by name: %s (type: %s)", result.getName(), result.getType())
                else:
                    # If no match by name, try partial matching on type
                    self.logger.info("No match by name, trying partial matching on type...")
                    for nodedef in all_node_defs:
                        nodedef_type = nodedef.getType()
                        if (node_type in nodedef_type or nodedef_type in node_type) and (
                            category is None or nodedef.getCategory() == category
                        ):
                            result = nodedef
                            self.logger.info(
                                "Found partial match by type: %s (type: %s)", nodedef.getName(), nodedef_type
                            )
                            break
                    else:
                        result = None
                        self.logger.warning("No node definition found for '%s' (category: %s)", node_type, category)

            # Cache the result
            if result is not None:
                self._node_def_cache[cache_key] = result

            self.performance_monitor.end_operation("get_node_definition")

        except Exception:
            self.logger.exception("Error looking up node definition %s", node_type)
            self.performance_monitor.end_operation("get_node_definition")
            return None

        else:
            return result

    def get_input_definition(self, node_type: str, input_name: str, category: str | None = None) -> mx.Input | None:
        """Get an input definition from a node definition.

        Args:
            node_type: The node type
            input_name: The input name
            category: Optional category filter

        Returns:
            mx.Input: The input definition or None if not found
        """
        nodedef = self.get_node_definition(node_type, category)
        if nodedef:
            # getActiveInput resolves inherited inputs (e.g. standard_surface only
            # declares base/base_color directly; the rest come via inheritance).
            return nodedef.getActiveInput(input_name) or nodedef.getInput(input_name)
        return None

    def get_output_definition(self, node_type: str, output_name: str, category: str | None = None) -> mx.Output | None:
        """Get an output definition from a node definition.

        Args:
            node_type: The node type
            output_name: The output name
            category: Optional category filter

        Returns:
            mx.Output: The output definition or None if not found
        """
        nodedef = self.get_node_definition(node_type, category)
        if nodedef:
            return nodedef.getOutput(output_name)
        return None

    def validate_document(self) -> bool:
        """Validate the MaterialX document with comprehensive validation.

        Returns:
            bool: True if document is valid
        """
        if not self.document:
            return False

        try:
            self.performance_monitor.start_operation("validate_document")

            # Use advanced validator for comprehensive validation
            validation_results = self.advanced_validator.validate_document_comprehensive(self.document)

            # Log validation summary
            summary = self.advanced_validator.get_validation_summary(validation_results)
            self.logger.info("Document validation: %s", summary)

            # Log detailed results
            if validation_results["errors"]:
                self.logger.error("Validation errors:")
                for error in validation_results["errors"]:
                    self.logger.error("  - %s", error)

            if validation_results["warnings"]:
                self.logger.warning("Validation warnings:")
                for warning in validation_results["warnings"]:
                    self.logger.warning("  - %s", warning)

            if validation_results["performance_issues"]:
                self.logger.warning("Performance issues:")
                for issue in validation_results["performance_issues"]:
                    self.logger.warning("  - %s", issue)

            if validation_results["suggestions"]:
                self.logger.info("Suggestions:")
                for suggestion in validation_results["suggestions"]:
                    self.logger.info("  - %s", suggestion)

            self.performance_monitor.end_operation("validate_document")
            return validation_results["valid"]

        except Exception:
            self.logger.exception("Document validation failed")
            self.performance_monitor.end_operation("validate_document")
            return False

    def _clear_caches(self):
        """Clear all caches to free memory."""
        self._node_def_cache.clear()
        self._input_def_cache.clear()
        self._output_def_cache.clear()
        gc.collect()

    def get_performance_stats(self) -> dict[str, Any]:
        """Get performance statistics."""
        return {
            "cache_sizes": {
                "node_defs": len(self._node_def_cache),
                "input_defs": len(self._input_def_cache),
                "output_defs": len(self._output_def_cache),
            },
            "suggestions": self.performance_monitor.suggest_optimizations(),
        }

    def cleanup(self):
        """Clean up resources and free memory."""
        self._clear_caches()
        self.performance_monitor.cleanup()
        self.logger.info("MaterialXDocumentManager cleanup completed")


class MaterialXTypeConverter:
    """Handles type conversion and validation for MaterialX inputs and outputs.

    This class provides:
    - Automatic type conversion between Blender and MaterialX types
    - Type validation and compatibility checking
    - Value formatting for different MaterialX types
    """

    def __init__(self, logger):
        self.logger = logger

        # Type compatibility mapping
        self.type_compatibility = {
            "color3": ["color3", "vector3"],
            "vector3": ["vector3", "color3"],
            "vector2": ["vector2"],
            "vector4": ["vector4", "color4"],
            "color4": ["color4", "vector4"],
            "float": ["float"],
            "filename": ["filename"],
            "string": ["string"],
            "integer": ["integer"],
            "boolean": ["boolean"],
        }

        # Blender to MaterialX type mapping
        self.blender_to_mtlx_types = {
            "RGBA": "color4",
            "RGB": "color3",
            "VECTOR": "vector3",
            "VECTOR_2D": "vector2",
            "VALUE": "float",
            "INT": "integer",
            "BOOLEAN": "boolean",
            "STRING": "string",
        }

    def convert_blender_type(self, blender_type: str) -> str:
        """Convert a Blender socket type to MaterialX type.

        Args:
            blender_type: The Blender socket type

        Returns:
            str: The corresponding MaterialX type
        """
        return self.blender_to_mtlx_types.get(blender_type, "color3")

    def validate_type_compatibility(self, from_type: str, to_type: str) -> bool:  # noqa: PLR0911
        """Validate if a type conversion is compatible.

        Args:
            from_type: The source type
            to_type: The target type

        Returns:
            bool: True if conversion is compatible
        """
        # Direct match
        if from_type == to_type:
            return True

        # Check compatibility mapping
        if from_type in self.type_compatibility:
            compatible_types = self.type_compatibility[from_type]
            if to_type in compatible_types:
                return True

        # Special cases
        if from_type == "color3" and to_type == "vector3":
            return True
        if from_type == "vector3" and to_type == "color3":
            return True
        if from_type == "color4" and to_type == "vector4":
            return True
        if from_type == "vector4" and to_type == "color4":
            return True

        self.logger.warning("Type incompatibility: %s -> %s", from_type, to_type)
        return False

    def convert_value(self, value: Any, target_type: str) -> Any:  # noqa: PLR0911
        """Convert a value to the target MaterialX type.

        Args:
            value: The value to convert
            target_type: The target MaterialX type

        Returns:
            Any: The converted value
        """
        try:
            # Handle Blender's bpy_prop_array types
            if hasattr(value, "__len__") and not isinstance(value, (str, bytes)):
                # Convert Blender arrays to list of floats
                try:
                    if hasattr(value, "default_value"):
                        # Handle Blender socket default values
                        value = value.default_value

                    # Convert to list of floats
                    float_list = []
                    for i in range(len(value)):
                        try:
                            float_list.append(float(value[i]))
                        except (TypeError, ValueError, IndexError):
                            float_list.append(0.0)

                    # Now handle based on target type
                    if target_type == "float":
                        return float_list[0] if float_list else 0.0
                    if target_type == "integer":
                        return int(float_list[0]) if float_list else 0
                    if target_type == "boolean":
                        return bool(float_list[0]) if float_list else False
                    if target_type == "string":
                        return str(value)
                    if target_type in ("color3", "vector3"):
                        if len(float_list) >= 3:
                            return [float_list[0], float_list[1], float_list[2]]
                        if len(float_list) >= 1:
                            return [float_list[0], float_list[0], float_list[0]]
                        return [0.0, 0.0, 0.0]
                    if target_type == "vector2":
                        if len(float_list) >= 2:
                            return [float_list[0], float_list[1]]
                        if len(float_list) >= 1:
                            return [float_list[0], float_list[0]]
                        return [0.0, 0.0]
                    if target_type == "color4":
                        if len(float_list) >= 4:
                            return [float_list[0], float_list[1], float_list[2], float_list[3]]
                        if len(float_list) >= 3:
                            return [float_list[0], float_list[1], float_list[2], 1.0]
                        if len(float_list) >= 1:
                            return [float_list[0], float_list[0], float_list[0], 1.0]
                        return [0.0, 0.0, 0.0, 1.0]

                except Exception:
                    self.logger.exception("Error converting Blender array %s to type %s:", value, target_type)
                    # Fallback to default values
                    if target_type == "float":
                        return 0.0
                    if target_type == "integer":
                        return 0
                    if target_type == "boolean":
                        return False
                    if target_type == "string":
                        return str(value)
                    if target_type in ("color3", "vector3"):
                        return [0.0, 0.0, 0.0]
                    if target_type == "vector2":
                        return [0.0, 0.0]
                    if target_type == "color4":
                        return [0.0, 0.0, 0.0, 1.0]
                    return value
                else:
                    return float_list

            # Handle regular types (non-array)
            if target_type == "float":
                return float(value)
            if target_type == "integer":
                return int(value)
            if target_type == "boolean":
                return bool(value)
            if target_type == "string":
                return str(value)
            if target_type in ("color3", "vector3"):
                if isinstance(value, (list, tuple)):
                    if len(value) >= 3:
                        return [float(value[0]), float(value[1]), float(value[2])]
                    if len(value) == 1:
                        return [float(value[0]), float(value[0]), float(value[0])]
                else:
                    val = float(value)
                    return [val, val, val]
            elif target_type == "vector2":
                if isinstance(value, (list, tuple)):
                    if len(value) >= 2:
                        return [float(value[0]), float(value[1])]
                    if len(value) == 1:
                        return [float(value[0]), float(value[0])]
                else:
                    val = float(value)
                    return [val, val]
            elif target_type == "color4":
                if isinstance(value, (list, tuple)):
                    if len(value) >= 4:
                        return [float(value[0]), float(value[1]), float(value[2]), float(value[3])]
                    if len(value) >= 3:
                        return [float(value[0]), float(value[1]), float(value[2]), 1.0]
                    if len(value) == 1:
                        return [float(value[0]), float(value[0]), float(value[0]), 1.0]
                else:
                    val = float(value)
                    return [val, val, val, 1.0]

        except Exception:
            self.logger.exception("Error converting value %s to type %s", value, target_type)
            # Return safe defaults
            if target_type == "float":
                return 0.0
            if target_type == "integer":
                return 0
            if target_type == "boolean":
                return False
            if target_type == "string":
                return str(value)
            if target_type in ("color3", "vector3"):
                return [0.0, 0.0, 0.0]
            if target_type == "vector2":
                return [0.0, 0.0]
            if target_type == "color4":
                return [0.0, 0.0, 0.0, 1.0]
            return value
        else:
            return value

    def format_value_string(self, value: Any, value_type: str) -> str:  # noqa: PLR0911
        """Format a value as a MaterialX value string.

        Args:
            value: The value to format
            value_type: The MaterialX type

        Returns:
            str: The formatted value string
        """
        try:
            if isinstance(value, (int, float)):
                return f"{value:.4g}"
            if isinstance(value, (list, tuple)):
                # Handle vector/color types
                if value_type in ["color3", "vector3"] and len(value) >= 3:
                    return f"{value[0]:.4g},{value[1]:.4g},{value[2]:.4g}"
                if value_type in ["color4", "vector4"] and len(value) >= 4:
                    return f"{value[0]:.4g},{value[1]:.4g},{value[2]:.4g},{value[3]:.4g}"
                if value_type in ["vector2"] and len(value) >= 2:
                    return f"{value[0]:.4g},{value[1]:.4g}"
                return ",".join(f"{v:.4g}" for v in value)
            return str(value)

        except Exception:
            self.logger.exception("Error formatting value %s for type %s", value, value_type)
            return str(value)


class MaterialXNodeBuilder:
    """Handles node creation using MaterialX library.

    This class provides:
    - Type-safe node creation with proper node definitions
    - Input/output handling with automatic type conversion
    - Connection management using mtlxutils
    - Value formatting and validation
    - Nodegraph creation and management
    """

    def __init__(self, document_manager: MaterialXDocumentManager, logger):
        self.doc_manager = document_manager
        self.logger = logger
        self.node_counter = 0
        self.created_nodes = {}
        self.type_converter = MaterialXTypeConverter(logger)

    def add_node(
        self, node_type: str, name: str, category: str | None = None, parent: mx.Element = None
    ) -> mx.Node | None:
        """Add a node to the document using MaterialX library.

        Args:
            node_type: The node type (e.g., 'standard_surface', 'mix')
            name: The node name
            category: Optional category (e.g., 'surfaceshader', 'color3')
            parent: Parent element (defaults to document root)

        Returns:
            mx.Node: The created node or None if failed
        """
        try:
            if not parent:
                parent = self.doc_manager.document

            # Generate unique name if needed
            if not name:
                name = f"{node_type}_{self.node_counter}"
                self.node_counter += 1

            # Create valid child name
            valid_name = parent.createValidChildName(name)

            # Get node definition
            if category:
                nodedef = self.doc_manager.get_node_definition(node_type, category)
            else:
                nodedef = self.doc_manager.get_node_definition(node_type)

            if not nodedef:
                self.logger.warning("No node definition found for %s (category: %s)", node_type, category)
                return None

            # Create node instance
            node = parent.addNodeInstance(nodedef, valid_name)

            if node:
                self.created_nodes[valid_name] = node
                self.logger.debug("Created node: %s (type: %s)", valid_name, node_type)

            return node

        except Exception:
            self.logger.exception("Failed to create node %s of type %s", name, node_type)
            return None

        else:
            return node

    def add_nodegraph(self, name: str, parent: mx.Element = None) -> mx.NodeGraph | None:
        """Add a nodegraph to the document.

        Args:
            name: The nodegraph name
            parent: Parent element (defaults to document root)

        Returns:
            mx.NodeGraph: The created nodegraph or None if failed
        """
        try:
            if not parent:
                parent = self.doc_manager.document

            valid_name = parent.createValidChildName(name)
            nodegraph = parent.addChildOfCategory("nodegraph", valid_name)

            if nodegraph:
                self.created_nodes[valid_name] = nodegraph
                self.logger.debug("Created nodegraph: %s", valid_name)

        except Exception:
            self.logger.exception("Failed to create nodegraph %s", name)
            return None
        else:
            return nodegraph

    def create_mtlx_input(
        self,
        node: mx.Node,
        input_name: str,
        value: Any = None,
        nodename: str | None = None,
        node_type: str | None = None,
        category: str | None = None,
    ) -> mx.Input | None:
        """Create a MaterialX input with type-safe handling.

        Args:
            node: The target node
            input_name: The input name
            value: The input value (for constant inputs)
            nodename: The connected node name (for connections)
            node_type: The node type for definition lookup
            category: The node category for definition lookup

        Returns:
            mx.Input: The created input or None if failed
        """
        try:
            # Get input definition for type information
            input_def = None
            if node_type:
                input_def = self.doc_manager.get_input_definition(node_type, input_name, category)

            # Determine input type
            input_type = input_def.getType() if input_def else self._get_input_type_from_name(input_name)

            # Create input
            input_elem = node.addInput(input_name, input_type)

            if input_elem:
                if value is not None:
                    # Convert and set constant value
                    converted_value = self.type_converter.convert_value(value, input_type)
                    formatted_value = self.type_converter.format_value_string(converted_value, input_type)
                    input_elem.setValueString(formatted_value)
                    self.logger.debug("Set input %s = %s (type: %s)", input_name, formatted_value, input_type)
                elif nodename:
                    # Set connection
                    input_elem.setNodeName(nodename)
                    self.logger.debug("Connected input %s to %s", input_name, nodename)

            return input_elem

        except Exception:
            self.logger.exception("Failed to create input %s for node {node.getName()}", input_name)
            return None

        else:
            return input_elem

    def add_input(
        self,
        node: mx.Node,
        input_name: str,
        input_type: str,
        value: Any = None,
        nodename: str | None = None,
    ) -> mx.Input | None:
        """Add an input to a node (legacy method for backward compatibility).

        Args:
            node: The target node
            input_name: The input name
            input_type: The input type
            value: The input value (for constant inputs)
            nodename: The connected node name (for connections)

        Returns:
            mx.Input: The created input or None if failed
        """
        return self.create_mtlx_input(node, input_name, value=value, nodename=nodename, node_type=input_type)

    def add_output(self, nodegraph: mx.NodeGraph, name: str, output_type: str, nodename: str) -> mx.Output | None:
        """Add an output to a nodegraph.

        Args:
            nodegraph: The target nodegraph
            name: The output name
            output_type: The output type
            nodename: The connected node name

        Returns:
            mx.Output: The created output or None if failed
        """
        try:
            valid_name = nodegraph.createValidChildName(name)
            output = nodegraph.addOutput(valid_name, output_type)

            if output:
                output.setNodeName(nodename)
                self.logger.debug("Added output %s connected to %s", valid_name, nodename)

        except Exception:
            self.logger.exception("Failed to add output %s to nodegraph %s", name, nodegraph.getName())
            return None

        else:
            return output

    def connect_nodes(self, from_node: mx.Node, from_output: str, to_node: mx.Node, to_input: str) -> bool:
        """Connect two nodes with type validation.

        Args:
            from_node: The source node
            from_output: The source output name
            to_node: The target node
            to_input: The target input name

        Returns:
            bool: True if connection successful
        """
        try:
            # Validate that nodes exist
            if not from_node or not to_node:
                self.logger.warning("Invalid nodes for connection: %s -> %s", from_node, to_node)
                return False

            # Get output and input definitions for type checking (handle None gracefully)
            from_output_def = from_node.getActiveOutput(from_output) if from_node else None
            to_input_def = to_node.getActiveInput(to_input) if to_node else None

            # Type validation (only if definitions exist)
            if from_output_def and to_input_def:
                try:
                    from_type = from_output_def.getType()
                    to_type = to_input_def.getType()

                    # Validate type compatibility
                    if not self.type_converter.validate_type_compatibility(from_type, to_type):
                        self.logger.warning("Type mismatch in connection: %s -> %s", from_type, to_type)
                        # Don't return False here, try the connection anyway
                except Exception as type_error:
                    self.logger.debug("Type validation failed, proceeding with connection: %s", type_error)

            # Use direct MaterialX connection method
            try:
                # Debug: Check what inputs are available on the target node
                node_def = to_node.getNodeDef()
                if node_def:
                    available_inputs = [inp.getName() for inp in node_def.getInputs()]
                    self.logger.debug(
                        "Available inputs for %s (%s): %s",
                        to_node.getName(),
                        to_node.getType(),
                        available_inputs,
                    )

                # Create input if it doesn't exist
                input_port = to_node.addInputFromNodeDef(to_input)
                if input_port:
                    # Remove any existing value
                    input_port.removeAttribute("value")
                    # Set the connection
                    input_port.setAttribute("nodename", from_node.getName())
                    if from_output and from_output != "out":
                        input_port.setOutputString(from_output)
                    success = True
                    self.logger.debug(
                        "Direct connection successful: %s.%s -> %s.%s",
                        from_node.getName(),
                        from_output,
                        to_node.getName(),
                        to_input,
                    )
                else:
                    self.logger.warning("Failed to create input port: %s.%s", to_node.getName(), to_input)
                    success = False
            except Exception:
                self.logger.exception("Direct connection failed")
                success = False

            if success:
                self.logger.debug(
                    "Connected %s.%s -> %s.%s", from_node.getName(), from_output, to_node.getName(), to_input
                )
            else:
                self.logger.warning(
                    "Failed to connect %s.%s -> %s.%s", from_node.getName(), from_output, to_node.getName(), to_input
                )

        except Exception:
            self.logger.exception("Error connecting nodes")
            return False
        else:
            return success

    def _get_input_type_from_name(self, input_name: str) -> str:
        """Get the expected type for an input based on its name.

        Args:
            input_name: The input name

        Returns:
            str: The expected input type
        """
        # Common input type mappings
        type_mapping = {
            "texcoord": "vector2",
            "in": "color3",
            "in1": "color3",
            "in2": "color3",
            "a": "color3",
            "b": "color3",
            "factor": "float",
            "scale": "float",
            "strength": "float",
            "amount": "float",
            "pivot": "vector2",
            "translate": "vector2",
            "rotate": "float",
            "file": "filename",
            "default": "color3",
            "surfaceshader": "surfaceshader",
            "normal": "vector3",
            "tangent": "vector3",
            # Standard surface specific mappings
            "base": "float",
            "base_color": "color3",
            "metalness": "float",
            "specular": "float",
            "specular_color": "color3",
            "specular_roughness": "float",
            "specular_ior": "float",
            "transmission": "float",
            "transmission_color": "color3",
            "transmission_depth": "float",
            "transmission_scatter": "color3",
            "transmission_scatter_anisotropy": "float",
            "transmission_dispersion": "float",
            "transmission_extra_roughness": "float",
            "opacity": "color3",
            "emission": "float",
            "emission_color": "color3",
            "subsurface": "float",
            "subsurface_color": "color3",
            "subsurface_radius": "color3",
            "subsurface_scale": "float",
            "subsurface_anisotropy": "float",
            "sheen": "float",
            "sheen_color": "color3",
            "sheen_tint": "float",
            "sheen_roughness": "float",
            "coat": "float",
            "coat_color": "color3",
            "coat_roughness": "float",
            "coat_ior": "float",
            "coat_normal": "vector3",
            "anisotropic": "float",
            "anisotropic_rotation": "float",
            "anisotropic_direction": "vector3",
        }

        return type_mapping.get(input_name.lower(), "color3")


class MaterialXConnectionManager:
    """Manages node connections and input/output handling.

    This class provides:
    - Connection validation
    - Type checking
    - Input/output mapping
    - Connection optimization
    """

    def __init__(self, logger):
        self.logger = logger
        self.connections = []
        self.type_mapping = {
            "color3": ["color3", "vector3"],
            "vector3": ["vector3", "color3"],
            "vector2": ["vector2"],
            "float": ["float"],
            "filename": ["filename"],
            "string": ["string"],
        }

    def validate_connection(self, from_type: str, to_type: str) -> bool:
        """Validate if a connection between types is valid.

        Args:
            from_type: The source type
            to_type: The target type

        Returns:
            bool: True if connection is valid
        """
        # Direct type match
        if from_type == to_type:
            return True

        # Check type compatibility
        if from_type in self.type_mapping:
            compatible_types = self.type_mapping[from_type]
            if to_type in compatible_types:
                return True

        # Special cases
        if from_type == "color3" and to_type == "vector3":
            return True
        if from_type == "vector3" and to_type == "color3":
            return True

        self.logger.warning("Type mismatch: %s -> %s", from_type, to_type)
        return False

    def get_input_type(self, input_name: str, node_type: str) -> str:  # noqa: ARG002
        """Get the expected type for an input based on its name and node type.

        Args:
            input_name: The input name
            node_type: The node type

        Returns:
            str: The expected input type
        """
        # Common input type mappings
        type_mapping = {
            "texcoord": "vector2",
            "in": "color3",
            "in1": "color3",
            "in2": "color3",
            "a": "color3",
            "b": "color3",
            "factor": "float",
            "scale": "float",
            "strength": "float",
            "amount": "float",
            "pivot": "vector2",
            "translate": "vector2",
            "rotate": "float",
            "file": "filename",
            "default": "color3",
            "normal": "vector3",
            "tangent": "vector3",
            # Standard surface specific mappings
            "base": "float",
            "base_color": "color3",
            "metalness": "float",
            "specular": "float",
            "specular_color": "color3",
            "specular_roughness": "float",
            "specular_ior": "float",
            "transmission": "float",
            "transmission_color": "color3",
            "transmission_depth": "float",
            "transmission_scatter": "color3",
            "transmission_scatter_anisotropy": "float",
            "transmission_dispersion": "float",
            "transmission_extra_roughness": "float",
            "opacity": "color3",
            "emission": "float",
            "emission_color": "color3",
            "subsurface": "float",
            "subsurface_color": "color3",
            "subsurface_radius": "color3",
            "subsurface_scale": "float",
            "subsurface_anisotropy": "float",
            "sheen": "float",
            "sheen_color": "color3",
            "sheen_tint": "float",
            "sheen_roughness": "float",
            "coat": "float",
            "coat_color": "color3",
            "coat_roughness": "float",
            "coat_ior": "float",
            "coat_normal": "vector3",
            "anisotropic": "float",
            "anisotropic_rotation": "float",
            "anisotropic_direction": "vector3",
        }

        return type_mapping.get(input_name.lower(), "color3")

    def record_connection(self, from_node: str, from_output: str, to_node: str, to_input: str):
        """Record a connection for later analysis.

        Args:
            from_node: Source node name
            from_output: Source output name
            to_node: Target node name
            to_input: Target input name
        """
        connection = {"from_node": from_node, "from_output": from_output, "to_node": to_node, "to_input": to_input}
        self.connections.append(connection)

    def get_connection_count(self, node_name: str) -> int:
        """Get the number of connections for a node.

        Args:
            node_name: The node name

        Returns:
            int: The number of connections
        """
        count = 0
        for conn in self.connections:
            if conn["from_node"] == node_name or conn["to_node"] == node_name:
                count += 1
        return count


class MaterialXLibraryBuilder:
    """Builds MaterialX documents using the MaterialX library.

    This is the main builder class that replaces the manual XML generation
    with proper MaterialX library APIs and Phase 3 enhancements.
    """

    def __init__(self, material_name: str, logger, version: str = "1.39"):
        self.material_name = material_name
        self.version = version
        self.logger = logger

        # Initialize core components
        self.doc_manager = MaterialXDocumentManager(logger, version)
        self.node_builder = MaterialXNodeBuilder(self.doc_manager, logger)
        self.connection_manager = MaterialXConnectionManager(logger)

        # Create document
        self.document = self.doc_manager.create_document()

        # Track created elements
        self.material_node = None
        self.nodegraph = None
        self.surface_shader = None

        # Legacy compatibility
        self.nodes = {}  # For backward compatibility
        self.connections = []
        self.node_counter = 0

        # Phase 3 enhancements
        self.performance_monitor = self.doc_manager.performance_monitor
        self.advanced_validator = self.doc_manager.advanced_validator

        # Advanced file writing options
        self.write_options = {
            "skip_library_elements": True,
            "write_xinclude": False,
            "remove_layout": True,
            "format_output": True,
        }

    def add_node(self, node_type: str, name: str, node_type_category: str | None = None, **params) -> str:
        """Add a node to the nodegraph (legacy compatibility method).

        Args:
            node_type: The node type
            name: The node name
            node_type_category: The node category
            **params: Node parameters

        Returns:
            str: The created node name
        """
        # Ensure nodegraph exists
        if not self.nodegraph:
            self.nodegraph = self.node_builder.add_nodegraph(self.material_name, self.document)

        # Create node
        node = self.node_builder.add_node(node_type, name, node_type_category, self.nodegraph)

        if node:
            node_name = node.getName()
            self.nodes[node_name] = node

            # Add parameters as inputs using type-safe method
            for param_name, param_value in params.items():
                if param_value is not None:
                    self.node_builder.create_mtlx_input(
                        node, param_name, param_value, node_type=node_type, category=node_type_category
                    )

            return node_name
        # Fallback: create a placeholder node
        placeholder_name = f"placeholder_{node_type}_{self.node_counter}"
        self.node_counter += 1
        self.logger.warning("Created placeholder node: %s", placeholder_name)
        return placeholder_name

    def add_surface_shader_node(self, node_type: str, name: str, **params) -> str:
        """Add a surface shader node outside the nodegraph.

        Args:
            node_type: The node type
            name: The node name
            **params: Node parameters

        Returns:
            str: The created node name
        """
        # Create surface shader node at document level
        node = self.node_builder.add_node(node_type, name, "surfaceshader", self.document)

        if node:
            node_name = node.getName()
            self.nodes[node_name] = node
            self.surface_shader = node

            # Add parameters as inputs using type-safe method
            for param_name, param_value in params.items():
                if param_value is not None:
                    self.node_builder.create_mtlx_input(
                        node, param_name, param_value, node_type=node_type, category="surfaceshader"
                    )

            return node_name
        return f"surface_{node_type}_{self.node_counter}"

    def add_connection(self, from_node: str, from_output: str, to_node: str, to_input: str):
        """Add a connection between nodes.

        Args:
            from_node: Source node name
            from_output: Source output name
            to_node: Target node name
            to_input: Target input name
        """
        from_node_elem = self.nodes.get(from_node)
        to_node_elem = self.nodes.get(to_node)

        if from_node_elem and to_node_elem:
            success = self.node_builder.connect_nodes(from_node_elem, from_output, to_node_elem, to_input)
            if success:
                self.connection_manager.record_connection(from_node, from_output, to_node, to_input)
                self.connections.append((from_node, from_output, to_node, to_input))
        else:
            self.logger.warning("Connection failed: %s.%s -> %s.%s", from_node, from_output, to_node, to_input)

    def add_surface_shader_input(
        self,
        surface_node_name: str,
        input_name: str,
        input_type: str,
        nodegraph_name: str | None = None,
        nodename: str | None = None,
        value: str | None = None,
    ):
        """Add an input to a surface shader node.

        Args:
            surface_node_name: The surface shader node name
            input_name: The input name
            input_type: The input type
            nodegraph_name: The connected nodegraph name
            nodename: The connected node name
            value: The input value
        """
        surface_node = self.nodes.get(surface_node_name)
        if not surface_node:
            self.logger.warning("Surface shader node '%s' not found", surface_node_name)
            return

        if nodegraph_name:
            # Connect to nodegraph output
            if self.nodegraph:
                # Reuse the existing output if the caller already created one for this input
                # (avoids a duplicate "<name>2" output that points at the output name itself).
                if self.nodegraph.getOutput(input_name) is None:
                    self.node_builder.add_output(self.nodegraph, input_name, input_type, nodename or input_name)
                # Reference the nodegraph output via nodegraph+output attributes
                # ("nodename=graph.output" dot-syntax is rejected by validation).
                # Use the sanitized nodegraph name, not the raw material name.
                surface_input = surface_node.getInput(input_name) or surface_node.addInput(input_name, input_type)
                surface_input.removeAttribute("value")
                surface_input.setNodeGraphString(self.nodegraph.getName())
                surface_input.setOutputString(input_name)
        elif nodename:
            # Connect to specific node
            self.node_builder.create_mtlx_input(surface_node, input_name, nodename=nodename)
        elif value is not None:
            # Set constant value
            self.node_builder.create_mtlx_input(surface_node, input_name, value)

    def add_output(self, name: str, output_type: str, nodename: str):
        """Add an output node to the nodegraph.

        Args:
            name: The output name
            output_type: The output type
            nodename: The connected node name
        """
        if self.nodegraph:
            self.node_builder.add_output(self.nodegraph, name, output_type, nodename)

    def set_material_surface(self, surface_node_name: str):
        """Set the surface shader for the material.

        Args:
            surface_node_name: The surface shader node name
        """
        if not self.material_node:
            # Create material node
            self.material_node = self.node_builder.add_node(
                "surfacematerial", self.material_name, "material", self.document
            )

        if self.material_node and self.surface_shader:
            # Connect material to surface shader
            self.node_builder.create_mtlx_input(self.material_node, "surfaceshader", nodename=surface_node_name)

    def add_displacement_shader_node(self, name: str) -> str:
        """Add a displacement shader node at the document level.

        Args:
            name: The node name

        Returns:
            str: The created node name
        """
        node = self.node_builder.add_node("displacement", name, "displacementshader", self.document)
        if node:
            node_name = node.getName()
            self.nodes[node_name] = node
            return node_name
        return name

    def set_material_displacement(self, displacement_node_name: str):
        """Set the displacement shader for the material.

        Args:
            displacement_node_name: The displacement shader node name
        """
        if not self.material_node:
            # Create material node
            self.material_node = self.node_builder.add_node(
                "surfacematerial", self.material_name, "material", self.document
            )

        if self.material_node:
            # Connect material to displacement shader
            self.node_builder.create_mtlx_input(
                self.material_node,
                "displacementshader",
                nodename=displacement_node_name,
                node_type="surfacematerial",
                category="material",
            )

    def _get_param_type(self, value) -> str:
        """Determine the MaterialX type for a value.

        Args:
            value: The value to type-check

        Returns:
            str: The MaterialX type
        """
        if isinstance(value, (int, float)):
            return "float"
        if isinstance(value, (list, tuple)):
            if len(value) == 2:
                return "vector2"
            if len(value) == 3:
                return "color3"
            if len(value) == 4:
                return "color4"
        return "string"

    def to_string(self) -> str:
        """Convert the document to a formatted XML string with advanced options.

        Returns:
            str: The formatted XML string
        """
        try:
            self.performance_monitor.start_operation("to_string")

            # Apply advanced write options
            if self.write_options["remove_layout"]:
                mxf.MtlxFile.removeLayout(self.document)

            # Use custom predicate for library elements
            predicate = mxf.MtlxFile.skipLibraryElement if self.write_options["skip_library_elements"] else None

            # Use mtlxutils for writing with advanced options
            content = mxf.MtlxFile.writeDocumentToString(self.document, predicate)

            # Format output if requested
            if self.write_options["format_output"]:
                try:
                    import xml.dom.minidom

                    dom = xml.dom.minidom.parseString(content)  # noqa: S318
                    content = dom.toprettyxml(indent="  ")
                except Exception as format_error:
                    self.logger.warning("Failed to format XML output: %s", format_error)

            self.performance_monitor.end_operation("to_string")

        except Exception:
            self.logger.exception("Failed to convert document to string")
            self.performance_monitor.end_operation("to_string")
            return ""

        else:
            return content

    def write_to_file(self, filepath: str) -> bool:
        """Write the document to a file with advanced options and validation.

        Args:
            filepath: The output file path

        Returns:
            bool: True if successful
        """
        try:
            self.performance_monitor.start_operation("write_to_file")

            # Validate document before writing
            validation_results = self.advanced_validator.validate_document_comprehensive(self.document)
            if not validation_results["valid"]:
                self.logger.warning("Writing document with validation issues:")
                for error in validation_results["errors"]:
                    self.logger.warning("  - %s", error)

            # Apply advanced write options
            if self.write_options["remove_layout"]:
                mxf.MtlxFile.removeLayout(self.document)

            # Use custom predicate for library elements
            predicate = mxf.MtlxFile.skipLibraryElement if self.write_options["skip_library_elements"] else None

            # Use mtlxutils for writing with advanced options
            mxf.MtlxFile.writeDocumentToFile(self.document, filepath, predicate)

            # Verify file was written successfully
            if not os.path.exists(filepath):
                raise RuntimeError(f"File was not created: {filepath}")  # noqa: TRY301

            file_size = os.path.getsize(filepath)
            self.logger.info("Successfully wrote MaterialX document to: %s (%s bytes)", filepath, file_size)

            self.performance_monitor.end_operation("write_to_file")

        except Exception:
            self.logger.exception("Failed to write document to file")
            self.performance_monitor.end_operation("write_to_file")
            return False

        else:
            return True

    def validate(self) -> bool:
        """Validate the MaterialX document with comprehensive validation.

        Returns:
            bool: True if document is valid
        """
        return self.doc_manager.validate_document()

    def get_performance_stats(self) -> dict[str, Any]:
        """Get comprehensive performance statistics.

        Returns:
            Dict containing performance metrics
        """
        return self.doc_manager.get_performance_stats()

    def set_write_options(self, **options):
        """Set advanced file writing options.

        Args:
            **options: Writing options to set
        """
        self.write_options.update(options)
        self.logger.info("Updated write options: %s", self.write_options)

    def optimize_document(self) -> bool:
        """Optimize the MaterialX document for better performance.

        Returns:
            bool: True if optimization was successful
        """
        try:
            self.performance_monitor.start_operation("optimize_document")

            self.logger.info("Optimizing MaterialX document...")

            # Remove unused nodes
            unused_nodes = self.advanced_validator._find_unused_nodes(self.document)  # noqa: SLF001
            if unused_nodes:
                self.logger.info("Removing %s unused nodes", len(unused_nodes))
                for node in unused_nodes:
                    try:
                        node.getParent().removeChild(node.getName())
                    except Exception:
                        self.logger.exception("Failed to remove unused node %s", node.getName())

            # Clear caches to free memory
            self.doc_manager._clear_caches()  # noqa: SLF001

            # Validate after optimization
            validation_results = self.advanced_validator.validate_document_comprehensive(self.document)
            if not validation_results["valid"]:
                self.logger.warning("Document has validation issues after optimization")

            self.logger.info("Document optimization completed")

            self.performance_monitor.end_operation("optimize_document")

        except Exception:
            self.logger.exception("Document optimization failed")
            self.performance_monitor.end_operation("optimize_document")
            return False

        else:
            return True

    def cleanup(self):
        """Clean up resources and free memory."""
        self.doc_manager.cleanup()
        self.logger.info("MaterialXLibraryBuilder cleanup completed")
