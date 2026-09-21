#!/usr/bin/python3
"""Recalculate an XLSX using the system Python/LibreOffice UNO bridge."""
import argparse
from pathlib import Path
import signal
import subprocess
import tempfile
import time
import uuid

import uno


def properties(**values):
    result = []
    for name, value in values.items():
        item = uno.createUnoStruct("com.sun.star.beans.PropertyValue")
        item.Name, item.Value = name, value
        result.append(item)
    return tuple(result)


def recalculate(source, target):
    if not source.is_file():
        raise ValueError(f"Input does not exist: {source}")
    if target.exists() or source == target:
        raise ValueError("Choose a new output path; existing files are never overwritten")
    if source.suffix.lower() != ".xlsx" or target.suffix.lower() != ".xlsx":
        raise ValueError("Input and output must be .xlsx files")
    with tempfile.TemporaryDirectory(prefix="hermes-calc-") as directory:
        pipe = "hermes_calc_" + uuid.uuid4().hex
        process = subprocess.Popen([
            "libreoffice", "-env:UserInstallation=" + Path(directory).as_uri(),
            "--headless", "--nologo", "--nodefault", "--norestore",
            f"--accept=pipe,name={pipe};urp;StarOffice.ComponentContext",
        ], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, start_new_session=True)
        document = None
        try:
            local = uno.getComponentContext()
            resolver = local.ServiceManager.createInstanceWithContext(
                "com.sun.star.bridge.UnoUrlResolver", local)
            deadline = time.monotonic() + 20
            while True:
                try:
                    context = resolver.resolve(
                        f"uno:pipe,name={pipe};urp;StarOffice.ComponentContext")
                    break
                except uno.getClass("com.sun.star.connection.NoConnectException"):
                    if process.poll() is not None or time.monotonic() >= deadline:
                        raise RuntimeError("LibreOffice did not start within 20 seconds")
                    time.sleep(0.1)
            desktop = context.ServiceManager.createInstanceWithContext(
                "com.sun.star.frame.Desktop", context)
            document = desktop.loadComponentFromURL(source.as_uri(), "_blank", 0, properties(
                Hidden=True, ReadOnly=True,
                MacroExecutionMode=uno.getConstantByName(
                    "com.sun.star.document.MacroExecMode.NEVER_EXECUTE"),
                UpdateDocMode=uno.getConstantByName(
                    "com.sun.star.document.UpdateDocMode.NO_UPDATE")))
            if document is None or not document.supportsService(
                    "com.sun.star.sheet.SpreadsheetDocument"):
                raise ValueError("Input is not a readable spreadsheet")
            document.calculateAll()
            document.storeAsURL(target.as_uri(), properties(
                FilterName="Calc MS Excel 2007 XML", Overwrite=False))
        finally:
            try:
                if document is not None:
                    document.close(True)
            finally:
                if process.poll() is None:
                    import os
                    os.killpg(process.pid, signal.SIGTERM)
                    try:
                        process.wait(timeout=5)
                    except subprocess.TimeoutExpired:
                        os.killpg(process.pid, signal.SIGKILL)
                        process.wait()


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("input", type=Path)
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    recalculate(args.input.resolve(), args.output.resolve())
    print(args.output)
