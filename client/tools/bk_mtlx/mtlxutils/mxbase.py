"""Base MaterialX utilities.

- version checking

Requires: MaterialX package
"""

import MaterialX as mx  # noqa: N813


def haveVersion(major, minor, patch):
    """Check if the current version matches a given version."""
    imajor, iminor, ipatch = mx.getVersionIntegers()

    if major >= imajor:
        if major > imajor:
            return True
        if iminor >= minor:
            if iminor > minor:
                return True
            if ipatch >= patch:
                return True
    return False
