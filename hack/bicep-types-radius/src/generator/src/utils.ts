// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
import path from 'path';
import { createWriteStream } from 'fs';
import { readdir, stat, mkdir, rm, copyFile } from 'fs/promises';
import { series } from 'async';
import { spawn } from 'child_process';
import colors from 'colors';

export interface ILogger {
  out: (data: string) => void;
  err: (data: string) => void;
}

export const defaultLogger: ILogger = {
  out: data => process.stdout.write(data),
  err: data => process.stderr.write(data),
}

export async function copyRecursive(sourceBasePath: string, destinationBasePath: string): Promise<void> {
  for (const filePath of await findRecursive(sourceBasePath, () => true)) {
    const destinationPath = path.join(destinationBasePath, path.relative(sourceBasePath, filePath));

    await mkdir(path.dirname(destinationPath), { recursive: true });
    await copyFile(filePath, destinationPath);
  }
}

export async function findRecursive(basePath: string, filter: (name: string) => boolean): Promise<string[]> {
  let results: string[] = [];

  for (const subPathName of await readdir(basePath)) {
    const subPath = path.resolve(`${basePath}/${subPathName}`);

    const fileStat = await stat(subPath);
    if (fileStat.isDirectory()) {
      const pathResults = await findRecursive(subPath, filter);
      results = results.concat(pathResults);
      continue;
    }

    if (!fileStat.isFile()) {
      continue;
    }

    if (!filter(subPath)) {
      continue;
    }

    results.push(subPath);
  }

  return results;
}

export function executeCmd(logger: ILogger, verbose: boolean, cwd: string, cmd: string, args: string[]) : Promise<void> {
  return new Promise((resolve, reject) => {
    if (verbose) {
      logOut(logger, colors.green(`Executing: ${cmd} ${args.join(' ')}`));
    }

    const child = spawn(cmd, args, {
      cwd: cwd,
      windowsHide: true,
      shell: true,
    });

    child.stdout.on('data', data => logger.out(colors.grey(data.toString())));
    child.stderr.on('data', data => {
      const message = data.toString();
      logger.err(colors.red(message));
      if (message.indexOf('FATAL ERROR') > -1 && message.indexOf('Allocation failed - JavaScript heap out of memory') > -1) {
        reject('Child process has run out of memory');
      }
    });

    child.on('error', err => {
      reject(err);
    });
    child.on('exit', code => {
      if (code !== 0) {
        reject(`Exited with code ${code}`);
      } else {
        resolve();
      }
    });
  });
}

export function executeSynchronous<T>(asyncFunc: () => Promise<T>) {
  series(
    [asyncFunc],
    (error) => {
      if (error) {
        throw error;
      }
    });
}

export function lowerCaseCompare(a: string, b: string) {
  const aLower = a.toLowerCase();
  const bLower = b.toLowerCase();

  if (aLower === bLower) {
    return 0;
  }

  return aLower < bLower ? -1 : 1;
}

export function logOut(logger: ILogger, line: string) {
  logger.out(`${line}\n`);
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
export function logErr(logger: ILogger, line: any) {
  logger.err(`${line}\n`);
}

export async function getLogger(logFilePath: string): Promise<ILogger> {
  await rm(logFilePath, { force: true });
  const logFileStream = createWriteStream(logFilePath, { flags: 'a' });

  return {
    out: (data: string) => {
      process.stdout.write(data);
      logFileStream.write(colors.stripColors(data));
    },
    err: (data: string) => {
      process.stdout.write(data);
      logFileStream.write(colors.stripColors(data));
    },
  };
}